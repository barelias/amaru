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

	"github.com/Masterminds/semver/v3"
	"github.com/barelias/amaru/internal/types"
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

func (c *GitHubClient) apiRequest(ctx context.Context, path string) ([]byte, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/%s", c.Owner, c.Repo, path)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	token, err := c.Auth.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
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
func (c *GitHubClient) ListVersions(ctx context.Context, itemType, name string) ([]*semver.Version, error) {
	// Tag format: skill/research/1.0.0 or command/dev/bootstrap/2.0.0
	prefix := itemType + "/" + name + "/"

	// Fetch all tags matching the prefix
	path := fmt.Sprintf("git/matching-refs/tags/%s", prefix)
	body, err := c.apiRequest(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("listing versions for %s/%s: %w", itemType, name, err)
	}

	var refs []struct {
		Ref string `json:"ref"`
	}
	if err := json.Unmarshal(body, &refs); err != nil {
		return nil, fmt.Errorf("parsing tag refs: %w", err)
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

// DownloadFiles downloads all files for a specific version.
// If version is empty, downloads from the default branch.
func (c *GitHubClient) DownloadFiles(ctx context.Context, itemType, name, version string) ([]File, error) {
	ref := ""
	if version != "" {
		ref = fmt.Sprintf("%s/%s/%s", itemType, name, version)
	}

	// Determine the directory path in the repo (.amaru_registry/ prefix)
	dirPath := ".amaru_registry/" + types.ItemType(itemType).DirName() + "/" + name

	return c.downloadDirectory(ctx, dirPath, ref, "")
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
