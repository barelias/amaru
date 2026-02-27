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

// parseGitHubURL parses "github:org/repo" or "https://github.com/org/repo" into owner and repo.
func parseGitHubURL(url string) (string, string, error) {
	if strings.HasPrefix(url, "github:") {
		parts := strings.SplitN(strings.TrimPrefix(url, "github:"), "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return "", "", fmt.Errorf("invalid github URL format: %s (expected github:org/repo)", url)
		}
		return parts[0], parts[1], nil
	}
	if strings.HasPrefix(url, "https://github.com/") {
		trimmed := strings.TrimPrefix(url, "https://github.com/")
		trimmed = strings.TrimSuffix(trimmed, ".git")
		parts := strings.SplitN(trimmed, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return "", "", fmt.Errorf("invalid github URL format: %s", url)
		}
		return parts[0], parts[1], nil
	}
	return "", "", fmt.Errorf("unsupported URL format: %s (expected github:org/repo or https://github.com/org/repo)", url)
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
	data, err := c.getFileContent(ctx, "registry.json", "")
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
func (c *GitHubClient) DownloadFiles(ctx context.Context, itemType, name, version string) ([]File, error) {
	tag := fmt.Sprintf("%s/%s/%s", itemType, name, version)

	// Determine the directory path in the repo
	var dirPath string
	if itemType == "skill" {
		dirPath = "skills/" + name
	} else {
		dirPath = "commands/" + name
	}

	return c.downloadDirectory(ctx, dirPath, tag, "")
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
