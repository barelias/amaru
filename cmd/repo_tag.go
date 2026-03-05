package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/barelias/amaru/internal/registry"
	"github.com/barelias/amaru/internal/scaffold"
	"github.com/barelias/amaru/internal/types"
	"github.com/barelias/amaru/internal/ui"

	"github.com/spf13/cobra"
)

var (
	repoTagType string
	repoTagNote string
	repoTagPush bool
)

var repoTagCmd = &cobra.Command{
	Use:   "tag <name> <version>",
	Short: "Tag a new version of a registry item",
	Long:  "Update manifest and index, then create an annotated git tag for the item.",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRepoTag(args[0], args[1])
	},
}

func init() {
	repoTagCmd.Flags().StringVarP(&repoTagType, "type", "t", "skill", "Item type: skill, command, or agent")
	repoTagCmd.Flags().StringVarP(&repoTagNote, "note", "n", "", "Changelog note for this version")
	repoTagCmd.Flags().BoolVar(&repoTagPush, "push", false, "Push the tag to remote after creation")
	repoCmd.AddCommand(repoTagCmd)
}

func runRepoTag(name, versionStr string) error {
	// Validate version
	v, err := semver.NewVersion(versionStr)
	if err != nil {
		return fmt.Errorf("invalid semver %q: %w", versionStr, err)
	}
	versionStr = v.String() // Normalize

	itemType := types.ItemType(repoTagType)
	if itemType != types.Skill && itemType != types.Command && itemType != types.Agent {
		return fmt.Errorf("invalid item type %q: must be skill, command, or agent", repoTagType)
	}

	dir, err := scaffold.FindRegistryRoot(".")
	if err != nil {
		return err
	}

	// Verify git repo
	if _, err := exec.Command("git", "rev-parse", "--git-dir").Output(); err != nil {
		return fmt.Errorf("not a git repository (required for tagging)")
	}

	idx, err := scaffold.LoadLocalIndex(dir)
	if err != nil {
		return err
	}

	entries := idx.EntriesForType(itemType)
	if _, exists := entries[name]; !exists {
		return fmt.Errorf("%s %q not found in registry index", itemType.Singular(), name)
	}

	// Verify item directory and manifest exist
	itemDir := filepath.Join(dir, ".amaru_registry", itemType.DirName(), name)
	manifestPath := filepath.Join(itemDir, "manifest.json")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("reading manifest.json: %w (does the item exist on disk?)", err)
	}

	var manifest registry.ItemManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return fmt.Errorf("parsing manifest.json: %w", err)
	}

	// Check tag doesn't already exist
	tagName := fmt.Sprintf("%s/%s/%s", itemType.Singular(), name, versionStr)
	out, _ := exec.Command("git", "tag", "-l", tagName).Output()
	if strings.TrimSpace(string(out)) != "" {
		return fmt.Errorf("tag %q already exists", tagName)
	}

	// Update manifest.json
	manifest.Version = versionStr
	if repoTagNote != "" {
		manifest.Changelog = append(manifest.Changelog, registry.ChangelogEntry{
			Version: versionStr,
			Date:    time.Now().Format("2006-01-02"),
			Note:    repoTagNote,
		})
	}
	newManifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}
	newManifestData = append(newManifestData, '\n')
	if err := os.WriteFile(manifestPath, newManifestData, 0644); err != nil {
		return fmt.Errorf("writing manifest.json: %w", err)
	}

	// Update index
	entry := entries[name]
	entry.Latest = versionStr
	entries[name] = entry
	scaffold.SetEntriesForType(idx, itemType, entries)
	scaffold.TouchUpdatedAt(idx)

	if err := scaffold.SaveLocalIndex(dir, idx); err != nil {
		return err
	}

	// Git operations: stage, commit, tag
	gitAdd := exec.Command("git", "add", "amaru_registry.json", manifestPath)
	if out, err := gitAdd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add: %s", strings.TrimSpace(string(out)))
	}

	commitMsg := fmt.Sprintf("release: %s", tagName)
	gitCommit := exec.Command("git", "commit", "-m", commitMsg)
	if out, err := gitCommit.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit: %s", strings.TrimSpace(string(out)))
	}

	tagMsg := fmt.Sprintf("%s/%s v%s", itemType.Singular(), name, versionStr)
	gitTag := exec.Command("git", "tag", "-a", tagName, "-m", tagMsg)
	if out, err := gitTag.CombinedOutput(); err != nil {
		return fmt.Errorf("git tag: %s", strings.TrimSpace(string(out)))
	}

	ui.Check("Tagged %s %q as v%s", itemType.Singular(), name, versionStr)
	fmt.Printf("  Tag: %s\n", tagName)

	if repoTagPush {
		pushCmd := exec.Command("git", "push", "--follow-tags")
		if out, err := pushCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git push: %s", strings.TrimSpace(string(out)))
		}
		ui.Check("Pushed to remote")
	} else {
		fmt.Printf("\n  To push: git push --follow-tags\n")
	}

	return nil
}
