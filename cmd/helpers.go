package cmd

import (
	"context"
	"fmt"
	"sync"

	"github.com/useamaru/amaru/internal/manifest"
	"github.com/useamaru/amaru/internal/registry"
	"github.com/useamaru/amaru/internal/ui"
)

// loadManifest loads the manifest from the current directory.
func loadManifest() (*manifest.Manifest, error) {
	m, err := manifest.Load(".")
	if err != nil {
		return nil, fmt.Errorf("failed to load amaru.json: %w\nRun 'amaru init' to create one", err)
	}
	return m, nil
}

// loadLock loads the lock file from the current directory.
func loadLock() (*manifest.Lock, error) {
	return manifest.LoadLock(".")
}

// buildClients creates registry clients for all registries in the manifest.
// Authenticates each one and prints status.
// Each client is configured with a mirror factory so that mirrors listed in
// amaru_registry.json are automatically fetched and merged on FetchIndex.
func buildClients(ctx context.Context, m *manifest.Manifest, silent bool) (map[string]registry.Client, error) {
	if !silent {
		fmt.Println("Authenticating registries...")
	}

	// Mirror factory: creates unauthenticated clients for mirror URLs.
	mirrorFactory := func(url string) (registry.Client, error) {
		noAuth, err := registry.NewAuthenticator("none", "")
		if err != nil {
			return nil, err
		}
		return registry.NewGitHubClient(url, noAuth)
	}

	clients := make(map[string]registry.Client)
	for alias, regConf := range m.Registries {
		auth, err := registry.NewAuthenticator(regConf.Auth, alias)
		if err != nil {
			return nil, fmt.Errorf("registry %s: %w", alias, err)
		}

		client, err := registry.NewGitHubClient(regConf.URL, auth)
		if err != nil {
			return nil, fmt.Errorf("registry %s: %w", alias, err)
		}

		client.WithMirrorFactory(mirrorFactory)

		// Validate authentication
		if _, err := auth.Token(ctx); err != nil && regConf.Auth != "none" {
			return nil, fmt.Errorf("registry %s authentication failed: %w", alias, err)
		}

		clients[alias] = client

		if !silent {
			ui.Check("%s (%s) — via %s", alias, regConf.URL, auth.Method())
		}
	}

	return clients, nil
}

// maxParallelDownloads bounds how many registry items are fetched at once.
// Each item still fans out internally over its own files, so this multiplies
// with registry.maxConcurrent — 6 keeps a skillset install well inside
// GitHub's secondary rate limits while removing the per-item round-trip stall.
const maxParallelDownloads = 6

// downloadJob is one item to fetch from a registry, plus the slot its result
// lands in. Callers fill the request fields; downloadItems fills Files/Err.
type downloadJob struct {
	Client   registry.Client
	ItemType string
	Name     string
	Version  string

	Files []registry.File
	Err   error
}

// downloadItems fetches every job concurrently, in place.
//
// jobs is the batch to fetch; each job's Files/Err are populated on return.
// Installing stays with the caller and stays sequential — only the network
// waits are overlapped, so output order and lock writes remain deterministic.
func downloadItems(ctx context.Context, jobs []downloadJob) {
	if len(jobs) == 0 {
		return
	}

	sem := make(chan struct{}, maxParallelDownloads)
	var wg sync.WaitGroup

	for i := range jobs {
		wg.Add(1)
		go func(job *downloadJob) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				job.Err = ctx.Err()
				return
			}
			defer func() { <-sem }()

			job.Files, job.Err = job.Client.DownloadFiles(ctx, job.ItemType, job.Name, job.Version)
		}(&jobs[i])
	}
	wg.Wait()
}
