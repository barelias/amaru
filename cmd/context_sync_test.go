package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/useamaru/amaru/internal/manifest"
)

func TestSyncManifestContextNoMountsIsNoop(t *testing.T) {
	m := &manifest.Manifest{Registries: map[string]manifest.RegistryConfig{}}

	if err := syncManifestContext(context.Background(), m); err != nil {
		t.Fatalf("a manifest without context mounts must be a no-op: %v", err)
	}
}

func TestSyncManifestContextRejectsUnknownRegistry(t *testing.T) {
	m := &manifest.Manifest{
		Registries: map[string]manifest.RegistryConfig{},
		Context: manifest.ContextMounts{{
			Registry: "missing_registry",
			Project:  "visio-rfcs",
			Path:     "docs/toolbox/rfc",
		}},
	}

	err := syncManifestContext(context.Background(), m)
	if err == nil {
		t.Fatal("a mount pointing at an unconfigured registry must fail loudly")
	}
	if !strings.Contains(err.Error(), "missing_registry") {
		t.Errorf("error should name the offending registry, got: %v", err)
	}
}
