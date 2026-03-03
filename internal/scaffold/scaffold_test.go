package scaffold

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScaffoldRepo(t *testing.T) {
	dir := t.TempDir()
	err := ScaffoldRepo(RepoConfig{Dir: dir})
	if err != nil {
		t.Fatalf("ScaffoldRepo error: %v", err)
	}

	// Verify required directories
	for _, d := range []string{".amaru_registry/skills", ".amaru_registry/commands", ".amaru_registry/agents", ".amaru_registry/context", ".amaru_registry/.sparse-profiles"} {
		info, err := os.Stat(filepath.Join(dir, d))
		if err != nil {
			t.Errorf("expected directory %s: %v", d, err)
		} else if !info.IsDir() {
			t.Errorf("expected %s to be a directory", d)
		}
	}

	// Verify amaru_registry.json
	data, err := os.ReadFile(filepath.Join(dir, "amaru_registry.json"))
	if err != nil {
		t.Fatalf("reading amaru_registry.json: %v", err)
	}
	var idx map[string]interface{}
	if err := json.Unmarshal(data, &idx); err != nil {
		t.Fatalf("parsing amaru_registry.json: %v", err)
	}
	if _, ok := idx["skills"]; !ok {
		t.Error("amaru_registry.json missing skills key")
	}
	if _, ok := idx["amaru_version"]; !ok {
		t.Error("amaru_registry.json missing amaru_version key")
	}

	// Verify AGENTS.md
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); err != nil {
		t.Error("expected AGENTS.md")
	}

	// Verify .gitkeep files
	for _, d := range []string{".amaru_registry/skills", ".amaru_registry/commands", ".amaru_registry/agents"} {
		if _, err := os.Stat(filepath.Join(dir, d, ".gitkeep")); err != nil {
			t.Errorf("expected .gitkeep in %s", d)
		}
	}
}

func TestScaffoldRepoWithProject(t *testing.T) {
	dir := t.TempDir()
	err := ScaffoldRepo(RepoConfig{Dir: dir, Project: "myapp"})
	if err != nil {
		t.Fatalf("ScaffoldRepo error: %v", err)
	}

	// Verify project-specific directories
	for _, d := range []string{
		".amaru_registry/context/myapp/brainstorms",
		".amaru_registry/context/myapp/plans",
		".amaru_registry/context/myapp/solutions",
	} {
		if _, err := os.Stat(filepath.Join(dir, d)); err != nil {
			t.Errorf("expected directory %s: %v", d, err)
		}
	}

	// Verify project AGENTS.md
	data, err := os.ReadFile(filepath.Join(dir, ".amaru_registry", "context", "myapp", "AGENTS.md"))
	if err != nil {
		t.Fatalf("reading project AGENTS.md: %v", err)
	}
	if !strings.Contains(string(data), "myapp") {
		t.Error("project AGENTS.md should reference project name")
	}

	// Verify sparse profile
	data, err = os.ReadFile(filepath.Join(dir, ".amaru_registry", ".sparse-profiles", "myapp"))
	if err != nil {
		t.Fatalf("reading sparse profile: %v", err)
	}
	if !strings.Contains(string(data), ".amaru_registry/context/myapp") {
		t.Error("sparse profile should reference project context path")
	}
}

func TestRootAgentsMD(t *testing.T) {
	content := RootAgentsMD()
	if content == "" {
		t.Fatal("expected non-empty content")
	}
	if !strings.Contains(content, "Registry Structure") {
		t.Error("expected Registry Structure heading")
	}
}

func TestProjectAgentsMD(t *testing.T) {
	content := ProjectAgentsMD("myapp")
	if !strings.Contains(content, "myapp") {
		t.Error("expected project name in content")
	}
	if !strings.Contains(content, "brainstorms") {
		t.Error("expected brainstorms section")
	}
}

func TestSparseProfile(t *testing.T) {
	content := SparseProfile("myapp")
	if !strings.Contains(content, ".amaru_registry/context/myapp") {
		t.Error("expected .amaru_registry/context/myapp path")
	}
	if !strings.Contains(content, "[include]") {
		t.Error("expected [include] section")
	}
	if !strings.Contains(content, "[exclude]") {
		t.Error("expected [exclude] section")
	}
}
