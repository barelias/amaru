package registry

import (
	"testing"

	"github.com/barelias/amaru/internal/types"
)

func TestParseGitHubURL(t *testing.T) {
	tests := []struct {
		url       string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{
			url:       "github:acme-org/acme-skills",
			wantOwner: "acme-org",
			wantRepo:  "acme-skills",
		},
		{
			url:       "https://github.com/acme-org/platform-skills",
			wantOwner: "acme-org",
			wantRepo:  "platform-skills",
		},
		{
			url:       "https://github.com/acme-org/platform-skills.git",
			wantOwner: "acme-org",
			wantRepo:  "platform-skills",
		},
		{
			url:     "github:invalid",
			wantErr: true,
		},
		{
			url:     "gitlab:org/repo",
			wantErr: true,
		},
		{
			url:     "github:/repo",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			owner, repo, err := parseGitHubURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseGitHubURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if owner != tt.wantOwner {
					t.Errorf("owner = %s, want %s", owner, tt.wantOwner)
				}
				if repo != tt.wantRepo {
					t.Errorf("repo = %s, want %s", repo, tt.wantRepo)
				}
			}
		})
	}
}

func TestNewAuthenticator(t *testing.T) {
	tests := []struct {
		method     string
		wantMethod string
		wantErr    bool
	}{
		{"github", "gh CLI", false},
		{"token", "env token", false},
		{"none", "none", false},
		{"oauth", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			auth, err := NewAuthenticator(tt.method, "main")
			if (err != nil) != tt.wantErr {
				t.Errorf("NewAuthenticator(%q) error = %v, wantErr %v", tt.method, err, tt.wantErr)
				return
			}
			if !tt.wantErr && auth.Method() != tt.wantMethod {
				t.Errorf("Method() = %s, want %s", auth.Method(), tt.wantMethod)
			}
		})
	}
}

func TestRegistryIndexEntriesForType(t *testing.T) {
	idx := &RegistryIndex{
		Skills:   map[string]RegistryEntry{"research": {Latest: "1.0.0"}},
		Commands: map[string]RegistryEntry{"bootstrap": {Latest: "2.0.0"}},
		Agents:   map[string]RegistryEntry{"coder": {Latest: "1.0.0"}},
	}

	tests := []struct {
		name     string
		itemType types.ItemType
		wantKey  string
	}{
		{"skill", types.Skill, "research"},
		{"command", types.Command, "bootstrap"},
		{"agent", types.Agent, "coder"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries := idx.EntriesForType(tt.itemType)
			if entries == nil {
				t.Fatal("expected non-nil entries")
			}
			if _, ok := entries[tt.wantKey]; !ok {
				t.Errorf("expected key %s", tt.wantKey)
			}
		})
	}

	if idx.EntriesForType(types.ItemType("widget")) != nil {
		t.Error("expected nil for unknown type")
	}
}
