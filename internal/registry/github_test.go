package registry

import (
	"testing"
)

func TestParseGitHubURL(t *testing.T) {
	tests := []struct {
		url       string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{
			url:       "github:acme-org/acme-skills",
			wantOwner: "acme-org",
			wantRepo:  "acme-skills",
		},
		{
			url:       "https://github.com/acme-org/platform-skills",
			wantOwner: "acme-org",
			wantRepo:  "platform-skills",
		},
		{
			url:       "https://github.com/acme-org/platform-skills.git",
			wantOwner: "acme-org",
			wantRepo:  "platform-skills",
		},
		{
			url:     "github:invalid",
			wantErr: true,
		},
		{
			url:     "gitlab:org/repo",
			wantErr: true,
		},
		{
			url:     "github:/repo",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			owner, repo, err := parseGitHubURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseGitHubURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if owner != tt.wantOwner {
					t.Errorf("owner = %s, want %s", owner, tt.wantOwner)
				}
				if repo != tt.wantRepo {
					t.Errorf("repo = %s, want %s", repo, tt.wantRepo)
				}
			}
		})
	}
}
