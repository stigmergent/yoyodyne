package config

import (
	"reflect"
	"slices"
	"testing"
)

func TestAddedTemplateKeysComparesEffectiveKeysRatherThanValuesOrAgentNames(t *testing.T) {
	for _, test := range []struct {
		name          string
		before, after string
		want          []string
	}{
		{
			name:   "merged new key",
			before: "agents: {developer: {role: developer, model: opus}}",
			after:  "agents: {developer: {<<: {role: developer, model: opus, effort: medium}}}",
			want:   []string{"agents.*.effort"},
		},
		{
			name:   "another agent and new values",
			before: "agents: {developer: {role: developer, effort: medium}}",
			after:  "agents: {developer: {role: developer, effort: high}, reviewer: {role: reviewer, effort: medium}}",
		},
		{
			name:   "overridden merge introduces no effective key",
			before: "agents: {developer: {role: developer}}",
			after:  "<<: {agents: {developer: {effort: medium}}}\nagents: {developer: {role: developer}}\n",
		},
		{
			name:   "checker does not know the landed key yet",
			before: "agents: {developer: {role: developer}}",
			after:  "agents: {developer: {role: developer, future_option: true}}",
			want:   []string{"agents.*.future_option"},
		},
		{
			name:   "sequence positions are not schema keys",
			before: "execution: {developer_slots: [{label: writing}]}",
			after:  "execution: {developer_slots: [{label: writing}, {label: reliability}]}",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := AddedTemplateKeys([]byte(test.before), []byte(test.after))
			if err != nil || !reflect.DeepEqual(got, test.want) {
				t.Fatalf("AddedTemplateKeys() = %v, %v; want %v", got, err, test.want)
			}
		})
	}
}

func TestUnreadableSchemaKeysNamesTheMissingParentOnce(t *testing.T) {
	older := slices.DeleteFunc(SchemaKeys(), func(key string) bool { return key == "agents.*.effort" })
	keys := []string{"agents.*.effort", "agents.*.role", "future", "future.option", "future.option.nested"}
	want := []string{"agents.*.effort", "future"}
	if got := UnreadableSchemaKeys(keys, older); !reflect.DeepEqual(got, want) {
		t.Fatalf("UnreadableSchemaKeys() = %v, want %v", got, want)
	}
}

func TestAddedTemplateKeysRejectsAnInvalidMerge(t *testing.T) {
	for _, sources := range [][2][]byte{
		{[]byte("agents: {developer: {<<: false}}"), []byte("version: 1")},
		{nil, []byte("agents: {developer: {<<: false}}")},
	} {
		if _, err := AddedTemplateKeys(sources[0], sources[1]); err == nil {
			t.Fatal("an invalid template merge was compared without an error")
		}
	}
}
