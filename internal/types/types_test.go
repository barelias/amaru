package types

import "testing"

func TestAllInstallableTypes(t *testing.T) {
	got := AllInstallableTypes()
	if len(got) != 3 {
		t.Fatalf("expected 3 types, got %d", len(got))
	}
	if got[0] != Skill || got[1] != Command || got[2] != Agent {
		t.Errorf("expected [skill command agent], got %v", got)
	}
}

func TestDirName(t *testing.T) {
	tests := []struct {
		itemType ItemType
		want     string
	}{
		{Skill, "skills"},
		{Command, "commands"},
		{Agent, "agents"},
		{ItemType("widget"), "widgets"},
	}

	for _, tt := range tests {
		t.Run(string(tt.itemType), func(t *testing.T) {
			got := tt.itemType.DirName()
			if got != tt.want {
				t.Errorf("DirName() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestSingular(t *testing.T) {
	tests := []struct {
		itemType ItemType
		want     string
	}{
		{Skill, "skill"},
		{Command, "command"},
		{Agent, "agent"},
	}

	for _, tt := range tests {
		t.Run(string(tt.itemType), func(t *testing.T) {
			got := tt.itemType.Singular()
			if got != tt.want {
				t.Errorf("Singular() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestPlural(t *testing.T) {
	tests := []struct {
		itemType ItemType
		want     string
	}{
		{Skill, "skills"},
		{Command, "commands"},
		{Agent, "agents"},
	}

	for _, tt := range tests {
		t.Run(string(tt.itemType), func(t *testing.T) {
			got := tt.itemType.Plural()
			if got != tt.want {
				t.Errorf("Plural() = %s, want %s", got, tt.want)
			}
		})
	}
}
