package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/barelias/amaru/internal/registry"
	"github.com/barelias/amaru/internal/scaffold"
)

// scaffoldTestRegistry creates a minimal registry in a temp dir for testing.
func scaffoldTestRegistry(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	idx := &registry.RegistryIndex{
		AmaruVersion: "1",
		UpdatedAt:    "2026-03-05",
		Skills:       map[string]registry.RegistryEntry{},
		Commands:     map[string]registry.RegistryEntry{},
		Agents:       map[string]registry.RegistryEntry{},
		Skillsets:    map[string]registry.SkillsetEntry{},
	}
	if err := scaffold.SaveLocalIndex(dir, idx); err != nil {
		t.Fatalf("saving index: %v", err)
	}

	// Create type directories
	for _, d := range []string{"skills", "commands", "agents"} {
		if err := os.MkdirAll(filepath.Join(dir, ".amaru_registry", d), 0755); err != nil {
			t.Fatalf("creating dir: %v", err)
		}
	}

	return dir
}

// scaffoldTestRegistryWithSkill creates a registry with one skill already added.
func scaffoldTestRegistryWithSkill(t *testing.T, name string) string {
	t.Helper()
	dir := scaffoldTestRegistry(t)

	idx, _ := scaffold.LoadLocalIndex(dir)
	idx.Skills[name] = registry.RegistryEntry{
		Latest:      "",
		Description: "A test skill",
		Tags:        []string{"test"},
	}
	scaffold.SaveLocalIndex(dir, idx)

	// Create skill directory and files
	skillDir := filepath.Join(dir, ".amaru_registry", "skills", name)
	os.MkdirAll(skillDir, 0755)

	manifest := registry.ItemManifest{
		Name:        name,
		Type:        "skill",
		Version:     "",
		Description: "A test skill",
		Author:      "test",
		Files:       []string{"skill.md"},
		Tags:        []string{"test"},
	}
	data, _ := json.MarshalIndent(manifest, "", "  ")
	os.WriteFile(filepath.Join(skillDir, "manifest.json"), data, 0644)
	os.WriteFile(filepath.Join(skillDir, "skill.md"), []byte("# test\n"), 0644)

	return dir
}

func TestRepoAddCreatesSkill(t *testing.T) {
	dir := scaffoldTestRegistry(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	// Reset flags
	repoAddType = "skill"
	repoAddDescription = "Test skill"
	repoAddAuthor = "tester"
	repoAddTags = "test,example"
	repoAddItems = ""

	if err := runRepoAdd("my-skill"); err != nil {
		t.Fatalf("runRepoAdd() error = %v", err)
	}

	// Verify directory was created
	skillDir := filepath.Join(dir, ".amaru_registry", "skills", "my-skill")
	if _, err := os.Stat(skillDir); os.IsNotExist(err) {
		t.Fatal("skill directory not created")
	}

	// Verify manifest.json
	data, err := os.ReadFile(filepath.Join(skillDir, "manifest.json"))
	if err != nil {
		t.Fatalf("reading manifest.json: %v", err)
	}
	var m registry.ItemManifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parsing manifest.json: %v", err)
	}
	if m.Name != "my-skill" {
		t.Errorf("manifest name = %q, want %q", m.Name, "my-skill")
	}
	if m.Type != "skill" {
		t.Errorf("manifest type = %q, want %q", m.Type, "skill")
	}
	if m.Author != "tester" {
		t.Errorf("manifest author = %q, want %q", m.Author, "tester")
	}

	// Verify content file exists
	if _, err := os.Stat(filepath.Join(skillDir, "skill.md")); os.IsNotExist(err) {
		t.Fatal("skill.md not created")
	}

	// Verify index was updated
	idx, err := scaffold.LoadLocalIndex(dir)
	if err != nil {
		t.Fatalf("loading index: %v", err)
	}
	entry, ok := idx.Skills["my-skill"]
	if !ok {
		t.Fatal("my-skill not in index")
	}
	if entry.Description != "Test skill" {
		t.Errorf("index description = %q, want %q", entry.Description, "Test skill")
	}
}

func TestRepoAddRejectsInvalidName(t *testing.T) {
	dir := scaffoldTestRegistry(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	repoAddType = "skill"
	repoAddDescription = ""
	repoAddAuthor = ""
	repoAddTags = ""
	repoAddItems = ""

	if err := runRepoAdd("Invalid Name!"); err == nil {
		t.Fatal("expected error for invalid name")
	}
}

func TestRepoAddRejectsDuplicate(t *testing.T) {
	dir := scaffoldTestRegistryWithSkill(t, "existing")
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	repoAddType = "skill"
	repoAddDescription = ""
	repoAddAuthor = ""
	repoAddTags = ""
	repoAddItems = ""

	if err := runRepoAdd("existing"); err == nil {
		t.Fatal("expected error for duplicate name")
	}
}

func TestRepoAddCommand(t *testing.T) {
	dir := scaffoldTestRegistry(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	repoAddType = "command"
	repoAddDescription = "Test command"
	repoAddAuthor = "tester"
	repoAddTags = ""
	repoAddItems = ""

	if err := runRepoAdd("my-cmd"); err != nil {
		t.Fatalf("runRepoAdd() error = %v", err)
	}

	// Verify command directory
	cmdDir := filepath.Join(dir, ".amaru_registry", "commands", "my-cmd")
	if _, err := os.Stat(filepath.Join(cmdDir, "command.md")); os.IsNotExist(err) {
		t.Fatal("command.md not created")
	}

	idx, _ := scaffold.LoadLocalIndex(dir)
	if _, ok := idx.Commands["my-cmd"]; !ok {
		t.Fatal("my-cmd not in index commands")
	}
}

func TestRepoAddSkillset(t *testing.T) {
	dir := scaffoldTestRegistryWithSkill(t, "foo")
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	repoAddType = "skillset"
	repoAddDescription = "Test skillset"
	repoAddAuthor = ""
	repoAddTags = ""
	repoAddItems = "skill/foo"

	if err := runRepoAdd("my-pack"); err != nil {
		t.Fatalf("runRepoAdd() error = %v", err)
	}

	idx, _ := scaffold.LoadLocalIndex(dir)
	ss, ok := idx.Skillsets["my-pack"]
	if !ok {
		t.Fatal("my-pack not in skillsets")
	}
	if len(ss.Items) != 1 {
		t.Errorf("skillset items count = %d, want 1", len(ss.Items))
	}
}

func TestRepoAddSkillsetRejectsMissingMember(t *testing.T) {
	dir := scaffoldTestRegistry(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	repoAddType = "skillset"
	repoAddDescription = ""
	repoAddAuthor = ""
	repoAddTags = ""
	repoAddItems = "skill/nonexistent"

	if err := runRepoAdd("bad-pack"); err == nil {
		t.Fatal("expected error for missing member")
	}
}

func TestRepoRemoveSkill(t *testing.T) {
	dir := scaffoldTestRegistryWithSkill(t, "to-remove")
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	repoRemoveType = "skill"
	repoRemoveForce = false

	if err := runRepoRemove("to-remove"); err != nil {
		t.Fatalf("runRepoRemove() error = %v", err)
	}

	// Verify removed from index
	idx, _ := scaffold.LoadLocalIndex(dir)
	if _, ok := idx.Skills["to-remove"]; ok {
		t.Fatal("to-remove still in index")
	}

	// Verify directory removed
	skillDir := filepath.Join(dir, ".amaru_registry", "skills", "to-remove")
	if _, err := os.Stat(skillDir); !os.IsNotExist(err) {
		t.Fatal("skill directory not removed")
	}
}

func TestRepoRemoveBlockedBySkillset(t *testing.T) {
	dir := scaffoldTestRegistryWithSkill(t, "member")
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	// Add a skillset referencing the skill
	idx, _ := scaffold.LoadLocalIndex(dir)
	idx.Skillsets["my-pack"] = registry.SkillsetEntry{
		Description: "test",
		Items:       []registry.SkillsetItem{{Type: "skill", Name: "member"}},
	}
	scaffold.SaveLocalIndex(dir, idx)

	repoRemoveType = "skill"
	repoRemoveForce = false

	if err := runRepoRemove("member"); err == nil {
		t.Fatal("expected error when removing item referenced by skillset")
	}

	// Force should work
	repoRemoveForce = true
	if err := runRepoRemove("member"); err != nil {
		t.Fatalf("forced remove error = %v", err)
	}
}

func TestRepoRemoveNotFound(t *testing.T) {
	dir := scaffoldTestRegistry(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	repoRemoveType = "skill"
	repoRemoveForce = false

	if err := runRepoRemove("nonexistent"); err == nil {
		t.Fatal("expected error for nonexistent item")
	}
}

func TestRepoValidateClean(t *testing.T) {
	dir := scaffoldTestRegistryWithSkill(t, "valid-skill")
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	if err := runRepoValidate(); err != nil {
		t.Fatalf("runRepoValidate() error = %v", err)
	}
}

func TestRepoValidateErrors(t *testing.T) {
	dir := scaffoldTestRegistry(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	// Add an entry with no matching directory
	idx, _ := scaffold.LoadLocalIndex(dir)
	idx.Skills["ghost"] = registry.RegistryEntry{Description: "missing directory"}
	scaffold.SaveLocalIndex(dir, idx)

	if err := runRepoValidate(); err == nil {
		t.Fatal("expected validation error for missing directory")
	}
}
