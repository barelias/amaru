package registry

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
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

// ghAuthenticator uses the GitHub CLI for authentication.
type ghAuthenticator struct{}

func (a *ghAuthenticator) Method() string { return "gh CLI" }

func (a *ghAuthenticator) Token(ctx context.Context) (string, error) {
	// Check if gh is installed and authenticated
	cmd := exec.CommandContext(ctx, "gh", "auth", "status")
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("gh CLI not authenticated. Run 'gh auth login' first: %w", err)
	}

	// Get the token
	cmd = exec.CommandContext(ctx, "gh", "auth", "token")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get gh auth token: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
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

func (a *noAuthenticator) Method() string                       { return "none" }
func (a *noAuthenticator) Token(ctx context.Context) (string, error) { return "", nil }
