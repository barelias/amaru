package registry

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// Authenticator provides an authentication token for registry access.
type Authenticator interface {
	// Token returns the authentication token, or empty string for no auth.
	Token(ctx context.Context) (string, error)
	// Method returns the auth method name.
	Method() string
}

// NewAuthenticator creates the appropriate authenticator based on the auth method.
func NewAuthenticator(method, registryAlias string) (Authenticator, error) {
	switch method {
	case "github":
		return &ghAuthenticator{}, nil
	case "token":
		return &envTokenAuthenticator{alias: registryAlias}, nil
	case "none":
		return &noAuthenticator{}, nil
	default:
		return nil, fmt.Errorf("unknown auth method: %s", method)
	}
}

// ghAuthenticator uses the GitHub CLI for authentication. Token is called once
// per API request — including from the parallel download goroutines — so the
// result is resolved once and memoized for the process' lifetime.
type ghAuthenticator struct {
	once  sync.Once
	token string
	err   error
}

func (a *ghAuthenticator) Method() string { return "gh CLI" }

// Token returns the GitHub token, resolving it at most once per process.
//
// Returns the token, or an error when no credential can be resolved.
func (a *ghAuthenticator) Token(ctx context.Context) (string, error) {
	a.once.Do(func() {
		a.token, a.err = resolveGitHubToken(ctx)
	})
	return a.token, a.err
}

// resolveGitHubToken reads the token from the environment when gh itself would
// (GH_TOKEN/GITHUB_TOKEN take precedence in gh), falling back to the CLI.
//
// Returns the token, or an error describing why no credential is available.
func resolveGitHubToken(ctx context.Context) (string, error) {
	for _, envVar := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
		if token := strings.TrimSpace(os.Getenv(envVar)); token != "" {
			return token, nil
		}
	}

	out, err := exec.CommandContext(ctx, "gh", "auth", "token").Output()
	if err != nil {
		// `gh auth status` writes the actionable diagnosis (no gh, logged out,
		// expired token); it costs a round trip, so it only runs on failure.
		status, _ := exec.CommandContext(ctx, "gh", "auth", "status").CombinedOutput()
		detail := strings.TrimSpace(string(status))
		if detail == "" {
			detail = err.Error()
		}
		return "", fmt.Errorf("gh CLI not authenticated. Run 'gh auth login' first: %s", detail)
	}

	token := strings.TrimSpace(string(out))
	if token == "" {
		return "", fmt.Errorf("gh CLI returned an empty token. Run 'gh auth login' first")
	}
	return token, nil
}

// envTokenAuthenticator reads the token from an environment variable.
type envTokenAuthenticator struct {
	alias string
}

func (a *envTokenAuthenticator) Method() string { return "env token" }

func (a *envTokenAuthenticator) Token(ctx context.Context) (string, error) {
	envVar := "AMARU_TOKEN_" + strings.ToUpper(a.alias)
	token := os.Getenv(envVar)
	if token == "" {
		return "", fmt.Errorf("environment variable %s not set", envVar)
	}
	return token, nil
}

// noAuthenticator provides no authentication.
type noAuthenticator struct{}

func (a *noAuthenticator) Method() string                            { return "none" }
func (a *noAuthenticator) Token(ctx context.Context) (string, error) { return "", nil }
