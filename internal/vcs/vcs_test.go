package vcs

import "testing"

func TestDetect(t *testing.T) {
	backend := Detect()
	if backend == nil {
		t.Fatal("expected non-nil backend")
	}
	name := backend.Name()
	if name != "git" && name != "sapling" {
		t.Errorf("expected git or sapling, got %s", name)
	}
}

func TestSaplingBackendName(t *testing.T) {
	b := &SaplingBackend{}
	if b.Name() != "sapling" {
		t.Errorf("expected sapling, got %s", b.Name())
	}
}

func TestGitBackendName(t *testing.T) {
	b := &GitBackend{}
	if b.Name() != "git" {
		t.Errorf("expected git, got %s", b.Name())
	}
}
