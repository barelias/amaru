package cmd

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/useamaru/amaru/internal/registry"
)

// blockingClient records concurrency and holds every download until released,
// so a serial implementation never reaches a peak above 1.
type blockingClient struct {
	release  chan struct{}
	inFlight atomic.Int32
	peak     atomic.Int32
	failOn   string
}

func (c *blockingClient) FetchIndex(ctx context.Context) (*registry.RegistryIndex, error) {
	return nil, nil
}

func (c *blockingClient) ListVersions(ctx context.Context, itemType, name string) ([]*semver.Version, error) {
	return nil, nil
}

func (c *blockingClient) FetchSkillsetManifest(ctx context.Context, name, version string) (*registry.SkillsetManifest, error) {
	return nil, nil
}

func (c *blockingClient) DownloadFiles(ctx context.Context, itemType, name, version string) ([]registry.File, error) {
	n := c.inFlight.Add(1)
	for {
		peak := c.peak.Load()
		if n <= peak || c.peak.CompareAndSwap(peak, n) {
			break
		}
	}
	defer c.inFlight.Add(-1)

	if c.release != nil {
		<-c.release
	}
	if name == c.failOn {
		return nil, fmt.Errorf("boom")
	}
	return []registry.File{{Path: "SKILL.md", Content: []byte(name)}}, nil
}

func TestDownloadItemsFillsResultsInOrder(t *testing.T) {
	client := &blockingClient{}
	jobs := make([]downloadJob, 4)
	for i := range jobs {
		jobs[i] = downloadJob{Client: client, ItemType: "skill", Name: fmt.Sprintf("skill-%d", i)}
	}

	downloadItems(context.Background(), jobs)

	for i, job := range jobs {
		if job.Err != nil {
			t.Fatalf("job %d: unexpected error: %v", i, job.Err)
		}
		want := fmt.Sprintf("skill-%d", i)
		if got := string(job.Files[0].Content); got != want {
			t.Errorf("job %d landed in the wrong slot: got %q, want %q", i, got, want)
		}
	}
}

func TestDownloadItemsKeepsErrorsPerJob(t *testing.T) {
	client := &blockingClient{failOn: "skill-1"}
	jobs := []downloadJob{
		{Client: client, ItemType: "skill", Name: "skill-0"},
		{Client: client, ItemType: "skill", Name: "skill-1"},
		{Client: client, ItemType: "skill", Name: "skill-2"},
	}

	downloadItems(context.Background(), jobs)

	if jobs[0].Err != nil || jobs[2].Err != nil {
		t.Errorf("healthy jobs should not carry an error: %v, %v", jobs[0].Err, jobs[2].Err)
	}
	if jobs[1].Err == nil {
		t.Error("failing job should carry its own error")
	}
	if len(jobs[0].Files) != 1 {
		t.Error("a failing sibling must not discard the other jobs' files")
	}
}

func TestDownloadItemsRunsInParallel(t *testing.T) {
	client := &blockingClient{release: make(chan struct{})}
	jobs := make([]downloadJob, maxParallelDownloads)
	for i := range jobs {
		jobs[i] = downloadJob{Client: client, ItemType: "skill", Name: fmt.Sprintf("skill-%d", i)}
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		downloadItems(context.Background(), jobs)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for client.inFlight.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	close(client.release)
	wg.Wait()

	if client.peak.Load() < 2 {
		t.Errorf("downloads ran serially: peak concurrency %d", client.peak.Load())
	}
	if client.peak.Load() > int32(maxParallelDownloads) {
		t.Errorf("concurrency %d exceeded the %d cap", client.peak.Load(), maxParallelDownloads)
	}
}

func TestDownloadItemsEmptyBatch(t *testing.T) {
	downloadItems(context.Background(), nil)
}
