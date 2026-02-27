package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDependencySpecUnmarshalShorthand(t *testing.T) {
	input := `"^1.0.0"`
	var spec DependencySpec
	if err := json.Unmarshal([]byte(input), &spec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.Version != "^1.0.0" {
		t.Errorf("expected version ^1.0.0, got %s", spec.Version)
	}
	if spec.Registry != "" {
		t.Errorf("expected empty registry, got %s", spec.Registry)
	}
}

func TestDependencySpecUnmarshalFullForm(t *testing.T) {
	input := `{"version": "^1.2.0", "registry": "main"}`
	var spec DependencySpec
	if err := json.Unmarshal([]byte(input), &spec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.Version != "^1.2.0" {
		t.Errorf("expected version ^1.2.0, got %s", spec.Version)
	}
	if spec.Registry != "main" {
		t.Errorf("expected registry main, got %s", spec.Registry)
	}
}

func TestDependencySpecMarshalShorthand(t *testing.T) {
	spec := DependencySpec{Version: "^1.0.0"}
	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != `"^1.0.0"` {
		t.Errorf("expected shorthand marshal, got %s", string(data))
	}
}

func TestDependencySpecMarshalFullForm(t *testing.T) {
	spec := DependencySpec{Version: "^1.2.0", Registry: "main"}
	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result map[string]string
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unexpected error unmarshaling result: %v", err)
	}
	if result["version"] != "^1.2.0" || result["registry"] != "main" {
		t.Errorf("unexpected full form: %s", string(data))
	}
}

func TestManifestLoadShorthand(t *testing.T) {
	dir := t.TempDir()
	content := `{
  "version": "1.0.0",
  "registries": {
    "main": { "url": "github:acme-org/acme-skills", "auth": "github" }
  },
  "skills": {
    "research": "^1.0.0",
    "plan": "^1.0.0"
  }
}`
	if err := os.WriteFile(filepath.Join(dir, ManifestFile), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	m, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Skills["research"].Version != "^1.0.0" {
		t.Errorf("expected research ^1.0.0, got %s", m.Skills["research"].Version)
	}
	if m.Skills["research"].Registry != "" {
		t.Errorf("expected empty registry for shorthand, got %s", m.Skills["research"].Registry)
	}
}

func TestManifestLoadMultiRegistry(t *testing.T) {
	dir := t.TempDir()
	content := `{
  "version": "1.0.0",
  "registries": {
    "main": { "url": "github:acme-org/acme-skills", "auth": "github" },
    "platform": { "url": "github:acme-org/platform-skills", "auth": "github" }
  },
  "skills": {
    "research": { "version": "^1.0.0", "registry": "main" },
    "deploycheck": { "version": "^1.0.0", "registry": "platform" }
  }
}`
	if err := os.WriteFile(filepath.Join(dir, ManifestFile), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	m, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Skills["deploycheck"].Registry != "platform" {
		t.Errorf("expected platform registry, got %s", m.Skills["deploycheck"].Registry)
	}
}

func TestManifestValidation(t *testing.T) {
	tests := []struct {
		name    string
		m       Manifest
		wantErr bool
	}{
		{
			name:    "empty version",
			m:       Manifest{Registries: map[string]RegistryConfig{"x": {URL: "github:a/b", Auth: "none"}}},
			wantErr: true,
		},
		{
			name:    "no registries",
			m:       Manifest{Version: "1.0.0", Registries: map[string]RegistryConfig{}},
			wantErr: true,
		},
		{
			name: "invalid auth",
			m: Manifest{
				Version:    "1.0.0",
				Registries: map[string]RegistryConfig{"x": {URL: "github:a/b", Auth: "oauth"}},
			},
			wantErr: true,
		},
		{
			name: "valid",
			m: Manifest{
				Version:    "1.0.0",
				Registries: map[string]RegistryConfig{"x": {URL: "github:a/b", Auth: "github"}},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.m.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestResolveRegistry(t *testing.T) {
	m := &Manifest{
		Version: "1.0.0",
		Registries: map[string]RegistryConfig{
			"main": {URL: "github:acme-org/acme-skills", Auth: "github"},
		},
	}

	// Shorthand should resolve to default
	alias, err := m.ResolveRegistry(DependencySpec{Version: "^1.0.0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if alias != "main" {
		t.Errorf("expected main, got %s", alias)
	}

	// Explicit registry
	alias, err = m.ResolveRegistry(DependencySpec{Version: "^1.0.0", Registry: "main"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if alias != "main" {
		t.Errorf("expected main, got %s", alias)
	}

	// Multi-registry without explicit should error
	m.Registries["platform"] = RegistryConfig{URL: "github:acme-org/platform-skills", Auth: "github"}
	_, err = m.ResolveRegistry(DependencySpec{Version: "^1.0.0"})
	if err == nil {
		t.Error("expected error for ambiguous registry")
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	m := &Manifest{
		Version: "1.0.0",
		Registries: map[string]RegistryConfig{
			"main": {URL: "github:acme-org/acme-skills", Auth: "github"},
		},
		Skills: map[string]DependencySpec{
			"research": {Version: "^1.0.0"},
			"plan":     {Version: "^1.0.0", Registry: "main"},
		},
		Commands: map[string]DependencySpec{
			"dev/bootstrap": {Version: "^2.0.0", Registry: "main"},
		},
	}

	if err := Save(dir, m); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if loaded.Skills["research"].Version != "^1.0.0" {
		t.Errorf("expected ^1.0.0, got %s", loaded.Skills["research"].Version)
	}
	if loaded.Commands["dev/bootstrap"].Registry != "main" {
		t.Errorf("expected main registry, got %s", loaded.Commands["dev/bootstrap"].Registry)
	}
}
