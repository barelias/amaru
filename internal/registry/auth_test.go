package registry

import (
	"context"
	"sync"
	"testing"
)

func TestGhAuthenticatorPrefersEnvToken(t *testing.T) {
	t.Setenv("GH_TOKEN", "from-gh-token")
	t.Setenv("GITHUB_TOKEN", "from-github-token")

	got, err := (&ghAuthenticator{}).Token(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "from-gh-token" {
		t.Errorf("got %q, want GH_TOKEN to win", got)
	}
}

func TestGhAuthenticatorFallsBackToGithubToken(t *testing.T) {
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "from-github-token")

	got, err := (&ghAuthenticator{}).Token(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "from-github-token" {
		t.Errorf("got %q, want GITHUB_TOKEN", got)
	}
}

// The token is resolved once per process: Token is called on every API request,
// including from the parallel download goroutines, and it used to shell out to
// `gh auth status` + `gh auth token` each time.
func TestGhAuthenticatorResolvesOnce(t *testing.T) {
	t.Setenv("GH_TOKEN", "first")

	auth := &ghAuthenticator{}
	first, err := auth.Token(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Setenv("GH_TOKEN", "second")
	again, err := auth.Token(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if first != "first" || again != "first" {
		t.Errorf("token was re-resolved: first %q, again %q", first, again)
	}
}

func TestGhAuthenticatorTokenIsConcurrencySafe(t *testing.T) {
	t.Setenv("GH_TOKEN", "shared")

	auth := &ghAuthenticator{}
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if got, err := auth.Token(context.Background()); err != nil || got != "shared" {
				t.Errorf("got %q, %v", got, err)
			}
		}()
	}
	wg.Wait()
}

func TestEnvTokenAuthenticator(t *testing.T) {
	t.Setenv("AMARU_TOKEN_PLATAFORMA", "secret")

	auth, err := NewAuthenticator("token", "plataforma")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := auth.Token(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "secret" {
		t.Errorf("got %q, want %q", got, "secret")
	}
}
