package hooks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallHookNew(t *testing.T) {
	dir := t.TempDir()
	hooksDir := filepath.Join(dir, HooksDir)
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatal(err)
	}

	script := PostCheckoutScript()
	if err := InstallHook(dir, "post-checkout", script); err != nil {
		t.Fatalf("InstallHook error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(hooksDir, "post-checkout"))
	if err != nil {
		t.Fatalf("reading hook: %v", err)
	}
	if !strings.Contains(string(data), "amaru:") {
		t.Error("hook should contain amaru marker")
	}

	// Verify executable permissions
	info, _ := os.Stat(filepath.Join(hooksDir, "post-checkout"))
	if info.Mode().Perm()&0100 == 0 {
		t.Error("hook should be executable")
	}
}

func TestInstallHookAppend(t *testing.T) {
	dir := t.TempDir()
	hooksDir := filepath.Join(dir, HooksDir)
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create an existing hook
	existing := "#!/bin/bash\necho 'existing hook'\n"
	hookPath := filepath.Join(hooksDir, "post-checkout")
	if err := os.WriteFile(hookPath, []byte(existing), 0755); err != nil {
		t.Fatal(err)
	}

	script := PostCheckoutScript()
	if err := InstallHook(dir, "post-checkout", script); err != nil {
		t.Fatalf("InstallHook error: %v", err)
	}

	data, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "existing hook") {
		t.Error("existing hook content should be preserved")
	}
	if !strings.Contains(content, "amaru:") {
		t.Error("amaru script should be appended")
	}
}

func TestInstallHookSkipDuplicate(t *testing.T) {
	dir := t.TempDir()
	hooksDir := filepath.Join(dir, HooksDir)
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatal(err)
	}

	script := PostCheckoutScript()

	// Install once
	if err := InstallHook(dir, "post-checkout", script); err != nil {
		t.Fatal(err)
	}

	// Install again — should be a no-op
	if err := InstallHook(dir, "post-checkout", script); err != nil {
		t.Fatalf("second InstallHook error: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(hooksDir, "post-checkout"))
	count := strings.Count(string(data), "amaru:")
	if count != 1 {
		t.Errorf("expected 1 amaru marker, got %d", count)
	}
}

func TestPostCheckoutScript(t *testing.T) {
	s := PostCheckoutScript()
	if s == "" {
		t.Fatal("expected non-empty script")
	}
	if !strings.Contains(s, "#!/bin/bash") {
		t.Error("script should have bash shebang")
	}
	if !strings.Contains(s, "amaru context sync") {
		t.Error("script should call amaru context sync")
	}
}

func TestPostCommitScript(t *testing.T) {
	s := PostCommitScript()
	if s == "" {
		t.Fatal("expected non-empty script")
	}
	if !strings.Contains(s, "#!/bin/bash") {
		t.Error("script should have bash shebang")
	}
	if !strings.Contains(s, "amaru context push") {
		t.Error("script should call amaru context push")
	}
}
