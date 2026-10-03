package config

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// AddedTemplateKeys names the keys a shipped template introduces, in schema
// terms: agent names become * and sequence positions become []. Changing a
// value or adding another agent with the same keys introduces no schema key.
// A nil before is a template newly added to the repository.
func AddedTemplateKeys(before, after []byte) ([]string, error) {
	previous, err := templateKeys(before)
	if err != nil {
		return nil, fmt.Errorf("read the previous template's keys: %w", err)
	}
	current, err := templateKeys(after)
	if err != nil {
		return nil, fmt.Errorf("read the landed template's keys: %w", err)
	}
	var added []string
	for key := range current {
		if !previous[key] {
			added = append(added, key)
		}
	}
	sort.Strings(added)
	return added, nil
}

func templateKeys(source []byte) (map[string]bool, error) {
	keys := map[string]bool{}
	if source == nil {
		return keys, nil
	}
	var root yaml.Node
	if err := yaml.Unmarshal(source, &root); err != nil {
		return nil, err
	}
	if root.Kind == 0 {
		return nil, errors.New("the template is empty")
	}
	patterns := schemaPatterns(SchemaKeys())
	var walk func(*yaml.Node, []string) error
	visiting := map[*yaml.Node]bool{}
	walk = func(node *yaml.Node, shape []string) error {
		for node.Kind == yaml.AliasNode && node.Alias != nil {
			node = node.Alias
		}
		if visiting[node] {
			return fmt.Errorf("recursive alias at %s", strings.Join(shape, "."))
		}
		visiting[node] = true
		defer delete(visiting, node)
		switch node.Kind {
		case yaml.MappingNode:
			fields, err := mappingFields(node)
			if err != nil {
				return err
			}
			for key, value := range fields {
				child, known := matchKey(shape, key, patterns)
				if !known {
					child = appendPath(shape, key)
				}
				keys[strings.Join(child, ".")] = true
				// The decoder accepts a known leaf whole; its data is not keys.
				if !known || hasChildren(child, patterns) {
					if err := walk(&value, child); err != nil {
						return err
					}
				}
			}
		case yaml.SequenceNode:
			child := appendPath(shape, SchemaItem)
			keys[strings.Join(child, ".")] = true
			for _, value := range node.Content {
				if err := walk(value, child); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(documentContent(&root), nil); err != nil {
		return nil, err
	}
	return keys, nil
}

// UnreadableSchemaKeys compares keys in schema terms with a running build's
// schema. A missing parent is named once, rather than every child it prevents
// the build from reading.
func UnreadableSchemaKeys(keys, schema []string) []string {
	patterns := schemaPatterns(schema)
	missing := map[string]bool{}
	for _, key := range keys {
		var shape, named []string
		for _, segment := range strings.Split(key, ".") {
			named = appendPath(named, segment)
			child, known := matchKey(shape, segment, patterns)
			if !known {
				missing[strings.Join(named, ".")] = true
				break
			}
			shape = child
			// Anything within a leaf is data rather than a configuration key.
			if !hasChildren(shape, patterns) {
				break
			}
		}
	}
	var unreadable []string
	for key := range missing {
		unreadable = append(unreadable, key)
	}
	sort.Strings(unreadable)
	return unreadable
}

func schemaPatterns(schema []string) [][]string {
	patterns := make([][]string, 0, len(schema))
	for _, key := range schema {
		patterns = append(patterns, strings.Split(key, "."))
	}
	return patterns
}
