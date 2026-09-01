package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/useamaru/amaru/internal/manifest"
	"github.com/useamaru/amaru/internal/registry"
)

// fakeSSClient implements registry.Client with a mutable index — enough to
// simulate a registry advancing between installs.
type fakeSSClient struct {
	idx       *registry.RegistryIndex
	files     map[string][]registry.File // key: "type/name/version"
	downloads []string
}

func (f *fakeSSClient) FetchIndex(ctx context.Context) (*registry.RegistryIndex, error) {
	return f.idx, nil
}

func (f *fakeSSClient) ListVersions(ctx context.Context, itemType, name string) ([]*semver.Version, error) {
	return nil, nil
}

func (f *fakeSSClient) DownloadFiles(ctx context.Context, itemType, name, version string) ([]registry.File, error) {
	key := fmt.Sprintf("%s/%s/%s", itemType, name, version)
	f.downloads = append(f.downloads, key)
	fs, ok := f.files[key]
	if !ok {
		return nil, fmt.Errorf("not found: %s", key)
	}
	return fs, nil
}

func (f *fakeSSClient) FetchSkillsetManifest(ctx context.Context, name, version string) (*registry.SkillsetManifest, error) {
	return nil, fmt.Errorf("not implemented in fake")
}

// chdirTemp moves the test into a temp dir (installSkillset operates on ".").
func chdirTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	return dir
}

func ssFixture() (*manifest.Manifest, *manifest.Lock, *fakeSSClient, map[string]registry.Client) {
	client := &fakeSSClient{
		idx: &registry.RegistryIndex{
			Skills: map[string]registry.RegistryEntry{
				"alpha": {Latest: "1.0.0"},
			},
			Skillsets: map[string]registry.SkillsetEntry{
				"ss": {
					Latest: "1.0.0",
					Items:  []registry.SkillsetItem{{Type: "skill", Name: "alpha"}},
				},
			},
		},
		files: map[string][]registry.File{
			"skill/alpha/1.0.0": {{Path: "SKILL.md", Content: []byte("alpha v1")}},
		},
	}
	m := &manifest.Manifest{
		Registries: map[string]manifest.RegistryConfig{
			"main": {URL: "github:acme/registry", Auth: "none"},
		},
		Skillsets: map[string]manifest.SkillsetSpec{
			"ss": {Version: "^1.0.0"},
		},
	}
	lock := &manifest.Lock{
		Skills:    map[string]manifest.LockedEntry{},
		Commands:  map[string]manifest.LockedEntry{},
		Agents:    map[string]manifest.LockedEntry{},
		Skillsets: map[string]manifest.LockedSkillset{},
	}
	return m, lock, client, map[string]registry.Client{"main": client}
}

func installedSkillContent(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(".claude", "skills", name, "SKILL.md"))
	if err != nil {
		return ""
	}
	return string(data)
}

func TestInstallSkillsetPicksUpMemberBump(t *testing.T) {
	chdirTemp(t)
	m, lock, client, clients := ssFixture()
	ctx := context.Background()

	if err := installSkillset(ctx, "ss", m.Skillsets["ss"], m, lock, clients); err != nil {
		t.Fatalf("first install: %v", err)
	}
	if got := installedSkillContent(t, "alpha"); got != "alpha v1" {
		t.Fatalf("expected v1 content installed, got %q", got)
	}
	digestV1 := lock.Skillsets["ss"].Digest

	// Content bump WITHOUT member-list change — the case that used to be
	// skipped forever ("already installed" by presence alone).
	client.idx.Skills["alpha"] = registry.RegistryEntry{Latest: "1.1.0"}
	client.files["skill/alpha/1.1.0"] = []registry.File{{Path: "SKILL.md", Content: []byte("alpha v2")}}

	if err := installSkillset(ctx, "ss", m.Skillsets["ss"], m, lock, clients); err != nil {
		t.Fatalf("second install: %v", err)
	}
	if got := installedSkillContent(t, "alpha"); got != "alpha v2" {
		t.Errorf("expected member bump installed, got %q", got)
	}
	if lock.Skills["alpha"].Version != "1.1.0" {
		t.Errorf("expected lock at 1.1.0, got %s", lock.Skills["alpha"].Version)
	}
	if lock.Skillsets["ss"].Digest == digestV1 {
		t.Error("expected the skillset digest to change with the member version")
	}

	// Third run: genuinely current — no new downloads.
	before := len(client.downloads)
	if err := installSkillset(ctx, "ss", m.Skillsets["ss"], m, lock, clients); err != nil {
		t.Fatalf("third install: %v", err)
	}
	if len(client.downloads) != before {
		t.Errorf("expected no downloads when current, got %v", client.downloads[before:])
	}
}

func TestInstallSkillsetConvergesStaleDisk(t *testing.T) {
	chdirTemp(t)
	m, lock, _, clients := ssFixture()
	ctx := context.Background()

	if err := installSkillset(ctx, "ss", m.Skillsets["ss"], m, lock, clients); err != nil {
		t.Fatalf("install: %v", err)
	}

	// Disk out of sync with the lock (edited here, or a lock that advanced on
	// another machine while .claude/skills stayed local): install converges.
	path := filepath.Join(".claude", "skills", "alpha", "SKILL.md")
	if err := os.WriteFile(path, []byte("stale or edited"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := installSkillset(ctx, "ss", m.Skillsets["ss"], m, lock, clients); err != nil {
		t.Fatalf("reinstall: %v", err)
	}
	if got := installedSkillContent(t, "alpha"); got != "alpha v1" {
		t.Errorf("expected converged content, got %q", got)
	}

	// The same divergence under 'amaru ignore' is accepted drift — untouched.
	m.Ignored = []string{"alpha"}
	if err := os.WriteFile(path, []byte("deliberate local edit"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := installSkillset(ctx, "ss", m.Skillsets["ss"], m, lock, clients); err != nil {
		t.Fatalf("reinstall with ignore: %v", err)
	}
	if got := installedSkillContent(t, "alpha"); got != "deliberate local edit" {
		t.Errorf("expected ignored edit kept, got %q", got)
	}
}

func TestUpdateSkillsetMembersPicksUpBump(t *testing.T) {
	chdirTemp(t)
	m, lock, client, clients := ssFixture()
	ctx := context.Background()

	if err := installSkillset(ctx, "ss", m.Skillsets["ss"], m, lock, clients); err != nil {
		t.Fatalf("install: %v", err)
	}
	digestV1 := lock.Skillsets["ss"].Digest

	client.idx.Skills["alpha"] = registry.RegistryEntry{Latest: "1.1.0"}
	client.files["skill/alpha/1.1.0"] = []registry.File{{Path: "SKILL.md", Content: []byte("alpha v2")}}

	changed, err := updateSkillsetMembers(ctx, "ss", m, lock, clients)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if changed != 1 {
		t.Errorf("expected 1 changed member, got %d", changed)
	}
	if got := installedSkillContent(t, "alpha"); got != "alpha v2" {
		t.Errorf("expected updated content, got %q", got)
	}
	if lock.Skillsets["ss"].Digest == digestV1 {
		t.Error("expected digest to change on member update")
	}

	// Nothing further to do.
	changed, err = updateSkillsetMembers(ctx, "ss", m, lock, clients)
	if err != nil {
		t.Fatalf("second update: %v", err)
	}
	if changed != 0 {
		t.Errorf("expected 0 changes when current, got %d", changed)
	}
}
