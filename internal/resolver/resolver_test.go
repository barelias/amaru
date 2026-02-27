package resolver

import (
	"testing"

	"github.com/Masterminds/semver/v3"
)

func versions(strs ...string) []*semver.Version {
	var vs []*semver.Version
	for _, s := range strs {
		v, _ := semver.NewVersion(s)
		vs = append(vs, v)
	}
	return vs
}

func TestResolveCaretRange(t *testing.T) {
	avail := versions("1.0.0", "1.0.3", "1.1.0", "2.0.0")

	v, err := Resolve("^1.0.0", avail)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.String() != "1.1.0" {
		t.Errorf("expected 1.1.0, got %s", v.String())
	}
}

func TestResolveTildeRange(t *testing.T) {
	avail := versions("1.1.0", "1.1.1", "1.2.0", "2.0.0")

	v, err := Resolve("~1.1.0", avail)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.String() != "1.1.1" {
		t.Errorf("expected 1.1.1, got %s", v.String())
	}
}

func TestResolveExact(t *testing.T) {
	avail := versions("1.0.0", "1.0.3", "1.1.0")

	v, err := Resolve("1.0.3", avail)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.String() != "1.0.3" {
		t.Errorf("expected 1.0.3, got %s", v.String())
	}
}

func TestResolveNoMatch(t *testing.T) {
	avail := versions("2.0.0", "3.0.0")

	_, err := Resolve("^1.0.0", avail)
	if err == nil {
		t.Error("expected error for no matching version")
	}
}

func TestResolveEmptyList(t *testing.T) {
	_, err := Resolve("^1.0.0", nil)
	if err == nil {
		t.Error("expected error for empty version list")
	}
}

func TestIsUpgradable(t *testing.T) {
	avail := versions("1.0.0", "1.0.3", "1.1.0")

	upgradable, newV, err := IsUpgradable("1.0.3", "^1.0.0", avail)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !upgradable {
		t.Error("expected upgradable")
	}
	if newV.String() != "1.1.0" {
		t.Errorf("expected 1.1.0, got %s", newV.String())
	}
}

func TestIsUpgradableAlreadyLatest(t *testing.T) {
	avail := versions("1.0.0", "1.1.0")

	upgradable, _, err := IsUpgradable("1.1.0", "^1.0.0", avail)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if upgradable {
		t.Error("expected not upgradable")
	}
}

func TestClassifyUpdate(t *testing.T) {
	tests := []struct {
		from, to string
		want     string
	}{
		{"1.0.0", "2.0.0", "major"},
		{"1.0.0", "1.1.0", "minor"},
		{"1.0.0", "1.0.1", "patch"},
		{"1.0.0", "1.0.0", "none"},
	}

	for _, tt := range tests {
		t.Run(tt.from+"→"+tt.to, func(t *testing.T) {
			got := ClassifyUpdate(tt.from, tt.to)
			if got != tt.want {
				t.Errorf("ClassifyUpdate(%s, %s) = %s, want %s", tt.from, tt.to, got, tt.want)
			}
		})
	}
}
