package registry

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/barelias/amaru/internal/types"
)

const (
	maxRetries    = 3
	retryBaseWait = 500 * time.Millisecond
)

// GitHubClient implements Client using the GitHub API.
type GitHubClient struct {
	Owner string
	Repo  string
	Auth  Authenticator
}

// NewGitHubClient creates a new GitHub registry client from a URL like "github:org/repo".
func NewGitHubClient(url string, auth Authenticator) (*GitHubClient, error) {
	owner, repo, err := parseGitHubURL(url)
	if err != nil {
		return nil, err
	}
	return &GitHubClient{
		Owner: owner,
		Repo:  repo,
		Auth:  auth,
	}, nil
}

// parseGitHubURL parses various GitHub URL formats into owner and repo.
// Supported formats:
//   - github:org/repo (canonical shorthand)
//   - https://github.com/org/repo[.git]
//   - http://github.com/org/repo[.git] (auto-upgraded to HTTPS)
//   - git@github.com:org/repo[.git] (SSH colon syntax)
//   - ssh://git@github.com/org/repo[.git] (SSH URL syntax)
//   - github.com/org/repo[.git] (bare domain)
func parseGitHubURL(rawURL string) (string, string, error) {
	lower := strings.ToLower(rawURL)

	// Canonical shorthand: github:org/repo
	if strings.HasPrefix(lower, "github:") {
		return extractOwnerRepo(rawURL[len("github:"):], rawURL)
	}

	// SSH colon syntax: git@github.com:org/repo[.git]
	if strings.HasPrefix(lower, "git@github.com:") {
		return extractOwnerRepo(rawURL[len("git@github.com:"):], rawURL)
	}

	// SSH URL syntax: ssh://git@github.com/org/repo[.git]
	if strings.HasPrefix(lower, "ssh://git@github.com/") {
		return extractOwnerRepo(rawURL[len("ssh://git@github.com/"):], rawURL)
	}

	// HTTP: auto-upgrade to HTTPS (fall through)
	if strings.HasPrefix(lower, "http://github.com/") {
		rawURL = "https://github.com/" + rawURL[len("http://github.com/"):]
		lower = strings.ToLower(rawURL)
	}

	// Bare domain: github.com/org/repo (fall through to HTTPS)
	if strings.HasPrefix(lower, "github.com/") {
		rawURL = "https://" + rawURL
		lower = strings.ToLower(rawURL)
	}

	// HTTPS: https://github.com/org/repo[.git]
	if strings.HasPrefix(lower, "https://github.com/") {
		return extractOwnerRepo(rawURL[len("https://github.com/"):], rawURL)
	}

	// Non-GitHub SSH hosts
	if strings.HasPrefix(lower, "git@") || strings.HasPrefix(lower, "ssh://") {
		return "", "", fmt.Errorf("unsupported URL format: %s (only GitHub URLs are supported)", rawURL)
	}

	return "", "", fmt.Errorf("unsupported URL format: %s (expected github:org/repo or https://github.com/org/repo)", rawURL)
}

// extractOwnerRepo extracts owner and repo from a "org/repo[.git]" path fragment.
// Rejects URLs with extra path segments (e.g., org/repo/tree/main).
func extractOwnerRepo(path, originalURL string) (string, string, error) {
	path = strings.TrimSuffix(path, ".git")
	path = strings.TrimRight(path, "/")
	parts := strings.SplitN(path, "/", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid github URL: %s (expected org/repo)", originalURL)
	}
	if len(parts) == 3 {
		return "", "", fmt.Errorf("invalid github URL: %s (unexpected path segments after org/repo)", originalURL)
	}
	return parts[0], parts[1], nil
}

// NormalizeURL converts any accepted GitHub URL format to the canonical "github:org/repo" form.
func NormalizeURL(url string) (string, error) {
	owner, repo, err := parseGitHubURL(url)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("github:%s/%s", owner, repo), nil
}

func isRetryable(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests ||
		statusCode == http.StatusBadGateway ||
		statusCode == http.StatusServiceUnavailable ||
		statusCode == http.StatusGatewayTimeout
}

func (c *GitHubClient) apiRequest(ctx context.Context, path string) ([]byte, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/%s", c.Owner, c.Repo, path)

	token, err := c.Auth.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}

	var lastErr error
	for attempt := range maxRetries {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/vnd.github.v3+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("API request failed: %w", err)
			if attempt < maxRetries-1 {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(retryBaseWait << attempt):
				}
			}
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("reading response: %w", err)
		}

		if resp.StatusCode == http.StatusOK {
			return body, nil
		}

		if !isRetryable(resp.StatusCode) {
			return nil, fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
		}

		lastErr = fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
		if attempt < maxRetries-1 {
			wait := retryBaseWait << attempt
			if resp.StatusCode == http.StatusTooManyRequests {
				if ra := resp.Header.Get("Retry-After"); ra != "" {
					if secs, err := time.ParseDuration(ra + "s"); err == nil {
						wait = secs
					}
				}
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
		}
	}

	return nil, lastErr
}

// FetchIndex downloads and parses the registry.json from the default branch.
func (c *GitHubClient) FetchIndex(ctx context.Context) (*RegistryIndex, error) {
	data, err := c.getFileContent(ctx, "amaru_registry.json", "")
	if err != nil {
		return nil, fmt.Errorf("fetching registry index: %w", err)
	}

	var index RegistryIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("parsing registry index: %w", err)
	}
	if index.Skills == nil {
		index.Skills = make(map[string]RegistryEntry)
	}
	if index.Commands == nil {
		index.Commands = make(map[string]RegistryEntry)
	}
	if index.Agents == nil {
		index.Agents = make(map[string]RegistryEntry)
	}
	if index.Skillsets == nil {
		index.Skillsets = make(map[string]SkillsetEntry)
	}
	return &index, nil
}

// ListVersions returns all available versions for an item by listing git tags.
// Returns an empty list (not an error) if the registry has no tags for this item.
func (c *GitHubClient) ListVersions(ctx context.Context, itemType, name string) ([]*semver.Version, error) {
	// Tag format: skill/research/1.0.0 or command/dev/bootstrap/2.0.0
	prefix := itemType + "/" + name + "/"

	// Fetch all tags matching the prefix
	path := fmt.Sprintf("git/matching-refs/tags/%s", prefix)
	body, err := c.apiRequest(ctx, path)
	if err != nil {
		// No tags is normal for registries that don't use per-item version tags
		return nil, nil
	}

	var refs []struct {
		Ref string `json:"ref"`
	}
	if err := json.Unmarshal(body, &refs); err != nil {
		return nil, nil // Treat parse failures as "no versions"
	}

	var versions []*semver.Version
	for _, ref := range refs {
		// ref.Ref is like "refs/tags/skill/research/1.0.3"
		tag := strings.TrimPrefix(ref.Ref, "refs/tags/")
		vStr := strings.TrimPrefix(tag, prefix)
		v, err := semver.NewVersion(vStr)
		if err != nil {
			continue // Skip non-semver tags
		}
		versions = append(versions, v)
	}

	sort.Sort(semver.Collection(versions))
	return versions, nil
}

// DownloadFiles downloads all files for a specific item.
// Always downloads from the default branch — the version parameter is recorded
// in the lock file for tracking but not used as a git ref, since registries
// are monorepos that don't necessarily use per-item version tags.
func (c *GitHubClient) DownloadFiles(ctx context.Context, itemType, name, version string) ([]File, error) {
	// Determine the directory path in the repo (.amaru_registry/ prefix)
	dirPath := ".amaru_registry/" + types.ItemType(itemType).DirName() + "/" + name

	return c.downloadDirectory(ctx, dirPath, "", "")
}

// FetchSkillsetManifest downloads the manifest.json for a skillset from the registry.
// Always fetches from the default branch — skillsets are metadata that reference
// individually versioned items, so they don't have their own version tags.
func (c *GitHubClient) FetchSkillsetManifest(ctx context.Context, name, _ string) (*SkillsetManifest, error) {
	filePath := ".amaru_registry/skillsets/" + name + "/manifest.json"
	data, err := c.getFileContent(ctx, filePath, "")
	if err != nil {
		return nil, fmt.Errorf("fetching skillset manifest for %q: %w", name, err)
	}

	var m SkillsetManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing skillset manifest for %q: %w", name, err)
	}

	return &m, nil
}

// downloadDirectory recursively downloads all files in a directory at a given ref.
func (c *GitHubClient) downloadDirectory(ctx context.Context, dirPath, ref, relativeBase string) ([]File, error) {
	path := fmt.Sprintf("contents/%s", dirPath)
	if ref != "" {
		path += "?ref=" + ref
	}

	body, err := c.apiRequest(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("listing directory %s: %w", dirPath, err)
	}

	var entries []struct {
		Name        string `json:"name"`
		Path        string `json:"path"`
		Type        string `json:"type"`
		Content     string `json:"content"`
		Encoding    string `json:"encoding"`
		DownloadURL string `json:"download_url"`
	}
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("parsing directory listing: %w", err)
	}

	var files []File
	for _, entry := range entries {
		relativePath := entry.Name
		if relativeBase != "" {
			relativePath = relativeBase + "/" + entry.Name
		}

		switch entry.Type {
		case "file":
			content, err := c.getFileContent(ctx, entry.Path, ref)
			if err != nil {
				return nil, fmt.Errorf("downloading %s: %w", entry.Path, err)
			}
			files = append(files, File{
				Path:    relativePath,
				Content: content,
			})
		case "dir":
			subFiles, err := c.downloadDirectory(ctx, entry.Path, ref, relativePath)
			if err != nil {
				return nil, err
			}
			files = append(files, subFiles...)
		}
	}

	return files, nil
}

// getFileContent fetches a file's content from the GitHub API.
func (c *GitHubClient) getFileContent(ctx context.Context, filePath, ref string) ([]byte, error) {
	path := fmt.Sprintf("contents/%s", filePath)
	if ref != "" {
		path += "?ref=" + ref
	}

	body, err := c.apiRequest(ctx, path)
	if err != nil {
		return nil, err
	}

	var fileResp struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if err := json.Unmarshal(body, &fileResp); err != nil {
		return nil, fmt.Errorf("parsing file response: %w", err)
	}

	if fileResp.Encoding != "base64" {
		return nil, fmt.Errorf("unexpected encoding: %s", fileResp.Encoding)
	}

	decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(fileResp.Content, "\n", ""))
	if err != nil {
		return nil, fmt.Errorf("decoding base64 content: %w", err)
	}
	return decoded, nil
}
