package config

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/mason-bryant/yoyodyne/internal/capability"
	"github.com/mason-bryant/yoyodyne/internal/domain"
)

func TestRoleDefinitionsLoadWithoutChangingAuthority(t *testing.T) {
	t.Parallel()
	project := t.TempDir()
	writeProject(t, project, minimalProjectConfig, nil)
	configPath := filepath.Join(project, DirectoryName, FileName)
	before, err := LoadResolved(configPath)
	if err != nil {
		t.Fatal(err)
	}
	body := "extends: architect\ntools:\n  add: [backlog.order]\n  remove: [repository.list]\n"
	source := writeRoleDefinition(t, filepath.Dir(configPath), "specialist", body)
	after, err := LoadResolved(configPath)
	if err != nil {
		t.Fatal(err)
	}
	definition, found := after.RoleDefinitions["specialist"]
	if !found || definition.Name != "specialist" || definition.Source != source || definition.Extends != domain.RoleArchitect {
		t.Fatalf("loaded role definition = %+v, found = %t", definition, found)
	}
	if !slices.Equal(definition.Tools.Add, []capability.Capability{capability.BacklogOrder}) ||
		!slices.Equal(definition.Tools.Remove, []capability.Capability{capability.RepositoryList}) {
		t.Fatalf("loaded tools = %+v", definition.Tools)
	}
	if want := fmt.Sprintf("%x", sha256.Sum256([]byte(body))); definition.Digest != want {
		t.Fatalf("digest = %q, want %q", definition.Digest, want)
	}
	if !reflect.DeepEqual(before.Config, after.Config) || before.Config.Revision() != after.Config.Revision() ||
		!reflect.DeepEqual(before.Origins, after.Origins) {
		t.Fatal("an unactivated role definition changed effective configuration or authority")
	}
	// Editing an inert file changes its digest, and still no effective authority.
	writeRoleDefinition(t, filepath.Dir(configPath), "specialist", body+"# edited\n")
	edited, err := LoadResolved(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if edited.RoleDefinitions["specialist"].Digest == definition.Digest || !reflect.DeepEqual(after.Config, edited.Config) {
		t.Fatal("editing a definition must change its digest and leave authority alone")
	}
}

func TestRoleDefinitionsRefuseInvalidFiles(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, body, want string
	}{
		{"missing base", "tools: {}\n", "extends must name exactly one shipped role"},
		{"empty file", "", "decode role definition"},
		{"null file", "null\n", "extends must name exactly one shipped role"},
		{"multiple bases", "extends: [architect, developer]\n", "cannot unmarshal"},
		{"unknown base", "extends: observer\n", `got "observer"`},
		{"custom base", "extends: another-definition\n", `got "another-definition"`},
		{"unknown addition", "extends: architect\ntools:\n  add: [repository.invented]\n", `"repository.invented"`},
		{"unknown removal", "extends: architect\ntools:\n  remove: [repository.invented]\n", `"repository.invented"`},
		{"gate evidence", "extends: architect\ntools:\n  add: [gate-evidence.mint]\n", `"gate-evidence.mint"`},
		{"human gate", "extends: architect\ntools:\n  add: [human-gate.record]\n", `"human-gate.record"`},
		{"unknown key", "extends: architect\ncapabilities: []\n", "field capabilities not found"},
		{"unknown tool key", "extends: architect\ntools:\n  grant: []\n", "field grant not found"},
		{"duplicate key", "extends: architect\nextends: developer\n", "already defined"},
		{"duplicate addition", "extends: architect\ntools:\n  add: [backlog.order, backlog.order]\n", `"backlog.order" more than once`},
		{"duplicate removal", "extends: architect\ntools:\n  remove: [repository.read, repository.read]\n", `"repository.read" more than once`},
		{"conflicting lists", "extends: architect\ntools:\n  add: [repository.read]\n  remove: [repository.read]\n", `"repository.read" is both added and removed`},
		{"unheld removal", "extends: architect\ntools:\n  remove: [backlog.order]\n", "the shipped architect does not hold"},
		{"multiple documents", "extends: architect\n---\nextends: developer\n", "exactly one YAML document"},
		{"trailing malformed document", "extends: architect\n---\n[\n", "decode role definition"},
		{"oversized", "extends: architect\n#" + strings.Repeat("x", MaxRoleDefinitionBytes), "exceeds the limit"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			project := t.TempDir()
			writeProject(t, project, minimalProjectConfig, nil)
			source := writeRoleDefinition(t, filepath.Join(project, DirectoryName), "specialist", test.body)
			resolved, err := LoadResolved(filepath.Join(project, DirectoryName, FileName))
			if err == nil || !strings.Contains(err.Error(), test.want) || !strings.Contains(err.Error(), source) {
				t.Fatalf("LoadResolved() error = %v, want %q with source %s", err, test.want, source)
			}
			if resolved.RoleDefinitions != nil || resolved.Config.Version != 0 {
				t.Fatal("a refused definition returned a partially loaded configuration")
			}
		})
	}
}

func TestRoleDefinitionsCannotAddRunOperations(t *testing.T) {
	t.Parallel()
	for _, primitive := range []capability.Capability{
		capability.ChecksExecute, capability.ReviewVerdict, capability.ForgePublish,
		capability.TargetBranchMutate, capability.PromotionLease, capability.WorktreeMutate,
		capability.ProviderInvoke, capability.RunStateMutate,
	} {
		t.Run(string(primitive), func(t *testing.T) {
			t.Parallel()
			project := t.TempDir()
			writeProject(t, project, minimalProjectConfig, nil)
			writeRoleDefinition(t, filepath.Join(project, DirectoryName), "specialist",
				fmt.Sprintf("extends: architect\ntools:\n  add: [%s]\n", primitive))
			_, err := Load(filepath.Join(project, DirectoryName, FileName))
			if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("tools.add names %q", primitive)) {
				t.Fatalf("Load() error = %v, want the forbidden primitive named", err)
			}
		})
	}
}

func TestRoleDefinitionsExtendEachShippedRole(t *testing.T) {
	t.Parallel()
	for _, role := range domain.Roles() {
		t.Run(string(role), func(t *testing.T) {
			t.Parallel()
			project := t.TempDir()
			writeProject(t, project, minimalProjectConfig, nil)
			writeRoleDefinition(t, filepath.Join(project, DirectoryName), "specialist", "extends: "+string(role)+"\n")
			definitions, err := LoadRoleDefinitions(filepath.Join(project, DirectoryName, FileName))
			if err != nil || definitions["specialist"].Extends != role {
				t.Fatalf("LoadRoleDefinitions() = %v, %v", definitions, err)
			}
		})
	}
}

func TestRoleDefinitionsCanRemoveInheritedRunAuthority(t *testing.T) {
	t.Parallel()
	project := t.TempDir()
	writeProject(t, project, minimalProjectConfig, nil)
	writeRoleDefinition(t, filepath.Join(project, DirectoryName), "specialist",
		"extends: developer\ntools:\n  remove: [checks.execute, forge.publish]\n")
	definitions, err := LoadRoleDefinitions(filepath.Join(project, DirectoryName, FileName))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(definitions["specialist"].Tools.Remove, []capability.Capability{capability.ChecksExecute, capability.ForgePublish}) {
		t.Fatal("the prohibition on additions also refused removal of inherited authority")
	}
}

func TestRoleDefinitionsDoNotBindAnAgentBeforeActivation(t *testing.T) {
	t.Parallel()
	project := t.TempDir()
	writeProject(t, project, minimalProjectConfig+"agents:\n  architect:\n    role: specialist\n", nil)
	writeRoleDefinition(t, filepath.Join(project, DirectoryName), "specialist", "extends: architect\n")
	_, err := LoadResolved(filepath.Join(project, DirectoryName, FileName))
	if err == nil || !strings.Contains(err.Error(), `unknown role "specialist"`) {
		t.Fatalf("LoadResolved() error = %v, want an inert definition refused as an agent's role", err)
	}
}

func TestRoleDefinitionsUseConfigurationDirectoryOrder(t *testing.T) {
	t.Parallel()
	project := t.TempDir()
	writeProject(t, project, minimalProjectConfig, nil)
	if err := os.WriteFile(filepath.Join(project, FileName), []byte(minimalProjectConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	writeRoleDefinition(t, project, "specialist", "extends: architect\n")
	// The lower-priority copy must not supply a base or hide a primary refusal.
	writeRoleDefinition(t, filepath.Join(project, DirectoryName), "specialist", "extends: observer\n")
	writeRoleDefinition(t, filepath.Join(project, DirectoryName), "fallback", "extends: developer\n")
	resolved, err := LoadResolved(filepath.Join(project, FileName))
	if err != nil || len(resolved.RoleDefinitions) != 2 || resolved.RoleDefinitions["specialist"].Extends != domain.RoleArchitect {
		t.Fatalf("LoadResolved() definitions = %v, error = %v", resolved.RoleDefinitions, err)
	}
	writeRoleDefinition(t, project, "specialist", "extends: observer\n")
	writeRoleDefinition(t, filepath.Join(project, DirectoryName), "specialist", "extends: architect\n")
	if _, err := LoadResolved(filepath.Join(project, FileName)); err == nil {
		t.Fatal("an invalid primary definition fell back to a valid secondary copy")
	}
	// Legacy configuration keeps definitions under .yoyodyne alone.
	definitions, err := LoadRoleDefinitions(filepath.Join(project, LegacyFileName))
	if err != nil || definitions["specialist"].Extends != domain.RoleArchitect {
		t.Fatalf("legacy definitions = %v, error = %v", definitions, err)
	}
}

func TestRoleDefinitionsRefuseBrokenPaths(t *testing.T) {
	t.Parallel()
	for _, shape := range []string{"directory escape", "file escape", "dangling directory", "dangling file", "directory file", "invalid name"} {
		t.Run(shape, func(t *testing.T) {
			t.Parallel()
			project := t.TempDir()
			writeProject(t, project, minimalProjectConfig, nil)
			root := filepath.Join(project, DirectoryName)
			outside := t.TempDir()
			source := writeRoleDefinition(t, outside, "specialist", "extends: architect\n")
			roles := filepath.Join(root, "roles")
			var err error
			switch shape {
			case "directory escape":
				err = os.Symlink(filepath.Dir(source), roles)
			case "dangling directory":
				err = os.Symlink(filepath.Join(outside, "missing"), roles)
			default:
				err = os.MkdirAll(roles, 0o700)
				if err == nil {
					switch shape {
					case "file escape":
						err = os.Symlink(source, filepath.Join(roles, "specialist.yaml"))
					case "dangling file":
						err = os.Symlink("missing", filepath.Join(roles, "specialist.yaml"))
					case "directory file":
						err = os.Mkdir(filepath.Join(roles, "specialist.yaml"), 0o700)
					case "invalid name":
						writeRoleDefinition(t, root, "Specialist", "extends: architect\n")
					}
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := LoadResolved(filepath.Join(root, FileName)); err == nil || !strings.Contains(err.Error(), root) {
				t.Fatalf("LoadResolved() error = %v, want broken role path named", err)
			}
		})
	}
}

func writeRoleDefinition(t *testing.T, directory, name, body string) string {
	t.Helper()
	roles := filepath.Join(directory, "roles")
	if err := os.MkdirAll(roles, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(roles, name+".yaml")
	if err := os.WriteFile(source, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return source
}
