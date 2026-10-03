package config

// Which keys a build can read, and which keys a file uses that it cannot.
//
// The decoder refuses a key it does not know, which is what makes a typo fail
// closed. It is also what makes a key added by a landing fatal to every
// process still running a build from before it: on 2026-09-28 the effort lines
// reached the main checkout while the dashboard ran a build two days older,
// and every read it made of the configuration failed with "field effort not
// found in type config.agentDocument" until somebody restarted it by hand. The
// file was right and the build was behind, and nothing said which.
//
// So a build can say which keys it reads — SchemaKeys, derived from the same
// document types the decoder fills, so it cannot drift from them — and a long
// running part records that list as it starts. What any later build can then
// ask is the question the decoder of the older one would answer by refusing:
// which keys of this file would that build not read. UnreadableKeys answers it
// from the recorded list alone, without the older build's types, because those
// are exactly what a newer build does not have.

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

const (
	// SchemaAnyKey stands in a key path for a mapping whose keys are names the
	// file chooses — an agent's name, an account's — rather than names the
	// build fixes.
	SchemaAnyKey = "*"
	// SchemaItem stands in a key path for an entry of a list.
	SchemaItem = "[]"
)

// SchemaKeys is every key path this build's configuration decoder accepts, in
// dotted form and sorted: `agents.*.effort`, `execution.developer_slots.[].label`.
// A path is listed whether it names a value or a section, so a reader can tell
// a section it knows from one it does not, and a value the build decodes as a
// whole — a duration — has nothing listed under it.
func SchemaKeys() []string {
	seen := map[string]struct{}{}
	walkSchema(reflect.TypeOf(configDocument{}), nil, seen, map[reflect.Type]bool{})
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

var (
	unmarshalerType         = reflect.TypeOf((*yaml.Unmarshaler)(nil)).Elem()
	obsoleteUnmarshalerType = reflect.TypeOf((*interface {
		UnmarshalYAML(func(any) error) error
	})(nil)).Elem()
	timeType = reflect.TypeOf(time.Time{})
)

// decodesWhole reports a type the decoder hands the whole node to rather than
// filling field by field, so nothing under it is a key the build names.
func decodesWhole(t reflect.Type) bool {
	if t == timeType {
		return true
	}
	return t.Implements(unmarshalerType) || reflect.PointerTo(t).Implements(unmarshalerType) ||
		t.Implements(obsoleteUnmarshalerType) || reflect.PointerTo(t).Implements(obsoleteUnmarshalerType)
}

func walkSchema(t reflect.Type, prefix []string, seen map[string]struct{}, visiting map[reflect.Type]bool) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if decodesWhole(t) {
		return
	}
	switch t.Kind() {
	case reflect.Struct:
		if visiting[t] {
			return
		}
		visiting[t] = true
		defer delete(visiting, t)
		for index := 0; index < t.NumField(); index++ {
			field := t.Field(index)
			if !field.IsExported() {
				continue
			}
			name, inline := yamlFieldName(field)
			if name == "-" {
				continue
			}
			if inline {
				walkSchema(field.Type, prefix, seen, visiting)
				continue
			}
			path := appendPath(prefix, name)
			seen[strings.Join(path, ".")] = struct{}{}
			walkSchema(field.Type, path, seen, visiting)
		}
	case reflect.Map:
		path := appendPath(prefix, SchemaAnyKey)
		seen[strings.Join(path, ".")] = struct{}{}
		walkSchema(t.Elem(), path, seen, visiting)
	case reflect.Slice, reflect.Array:
		path := appendPath(prefix, SchemaItem)
		seen[strings.Join(path, ".")] = struct{}{}
		walkSchema(t.Elem(), path, seen, visiting)
	}
}

// yamlFieldName is the key the decoder reads a field from, and whether the
// field's own keys are read as though they were the enclosing struct's.
func yamlFieldName(field reflect.StructField) (string, bool) {
	tag := field.Tag.Get("yaml")
	name, options, _ := strings.Cut(tag, ",")
	inline := false
	for _, option := range strings.Split(options, ",") {
		if option == "inline" {
			inline = true
		}
	}
	if name == "" {
		name = strings.ToLower(field.Name)
	}
	return name, inline
}

func appendPath(prefix []string, segment string) []string {
	path := make([]string, 0, len(prefix)+1)
	path = append(path, prefix...)
	return append(path, segment)
}

// UnreadableKeys is every key the configuration source uses that a build whose
// SchemaKeys were schema would refuse, as dotted paths with the file's own
// names in them — `agents.developer.effort` — sorted. A key a build does not
// know is listed and nothing under it is looked at, because it is the key the
// decoder stops on; a value the build decodes whole is not looked inside.
//
// It reads the file's shape only, never the values, and needs nothing of the
// build it asks about but the list: so a build can answer it for an older one
// it no longer has the types of.
func UnreadableKeys(source []byte, schema []string) ([]string, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(source, &root); err != nil {
		return nil, fmt.Errorf("read the configuration's keys: %w", err)
	}
	if root.Kind == 0 {
		return nil, errors.New("read the configuration's keys: the configuration is empty")
	}
	patterns := schemaPatterns(schema)
	var unreadable []string
	if err := walkFile(documentContent(&root), nil, nil, patterns, &unreadable); err != nil {
		return nil, fmt.Errorf("read the configuration's keys: %w", err)
	}
	sort.Strings(unreadable)
	return unreadable, nil
}

func documentContent(node *yaml.Node) *yaml.Node {
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		return node.Content[0]
	}
	return node
}

// walkFile descends one node of the file. shape is the path in the schema's
// terms, which is what is matched; named is the same path in the file's own
// names, which is what is reported.
func walkFile(node *yaml.Node, shape, named []string, patterns [][]string, unreadable *[]string) error {
	for node.Kind == yaml.AliasNode && node.Alias != nil {
		node = node.Alias
	}
	switch node.Kind {
	case yaml.MappingNode:
		fields, err := mappingFields(node)
		if err != nil {
			return err
		}
		for key, value := range fields {
			childNamed := appendPath(named, key)
			childShape, known := matchKey(shape, key, patterns)
			if !known {
				*unreadable = append(*unreadable, strings.Join(childNamed, "."))
				continue
			}
			if hasChildren(childShape, patterns) {
				if err := walkFile(&value, childShape, childNamed, patterns, unreadable); err != nil {
					return err
				}
			}
		}
	case yaml.SequenceNode:
		itemShape := appendPath(shape, SchemaItem)
		if !matches(itemShape, patterns) || !hasChildren(itemShape, patterns) {
			return nil
		}
		for index, item := range node.Content {
			itemNamed := appendPath(named, fmt.Sprintf("[%d]", index))
			if err := walkFile(item, itemShape, itemNamed, patterns, unreadable); err != nil {
				return err
			}
		}
	}
	return nil
}

// Decoding only the mapping expands merges with the loader's precedence:
// explicit keys override merged ones, and earlier merge sources win. Child
// nodes remain nodes, so keys are compared where their values are used.
func mappingFields(node *yaml.Node) (map[string]yaml.Node, error) {
	var fields map[string]yaml.Node
	if err := node.Decode(&fields); err != nil {
		return nil, err
	}
	return fields, nil
}

// matchKey finds the key under shape in the schema: by its own name where the
// build names it, or as a name the file chooses where the build reads a
// mapping there.
func matchKey(shape []string, key string, patterns [][]string) ([]string, bool) {
	if literal := appendPath(shape, key); matches(literal, patterns) {
		return literal, true
	}
	if chosen := appendPath(shape, SchemaAnyKey); matches(chosen, patterns) {
		return chosen, true
	}
	return nil, false
}

func matches(path []string, patterns [][]string) bool {
	for _, pattern := range patterns {
		if equalPath(pattern, path) {
			return true
		}
	}
	return false
}

func hasChildren(path []string, patterns [][]string) bool {
	for _, pattern := range patterns {
		if len(pattern) > len(path) && equalPath(pattern[:len(path)], path) {
			return true
		}
	}
	return false
}

func equalPath(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
