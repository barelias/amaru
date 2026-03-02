package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/barelias/amaru/internal/manifest"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Gera amaru.json inicial interativamente",
	Long:  "Cria um novo amaru.json com registries configurados interativamente.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInit()
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit() error {
	// Check if manifest already exists
	if _, err := os.Stat(manifest.ManifestFile); err == nil {
		return fmt.Errorf("amaru.json already exists. Delete it first to re-initialize")
	}

	reader := bufio.NewReader(os.Stdin)
	m := &manifest.Manifest{
		Version:    "1.0.0",
		Registries: make(map[string]manifest.RegistryConfig),
		Skills:     make(map[string]manifest.DependencySpec),
		Commands:   make(map[string]manifest.DependencySpec),
		Agents:     make(map[string]manifest.DependencySpec),
	}

	for {
		// Registry URL
		fmt.Print("Registry URL (ex: github:org/skills-repo): ")
		url, _ := reader.ReadString('\n')
		url = strings.TrimSpace(url)
		if url == "" {
			return fmt.Errorf("registry URL is required")
		}

		// Registry alias (suggest from URL)
		suggested := suggestAlias(url)
		fmt.Printf("Registry alias [%s]: ", suggested)
		alias, _ := reader.ReadString('\n')
		alias = strings.TrimSpace(alias)
		if alias == "" {
			alias = suggested
		}

		// Auth method
		fmt.Print("Auth method (github/token/none) [github]: ")
		auth, _ := reader.ReadString('\n')
		auth = strings.TrimSpace(auth)
		if auth == "" {
			auth = "github"
		}
		if auth != "github" && auth != "token" && auth != "none" {
			return fmt.Errorf("invalid auth method: %s", auth)
		}

		m.Registries[alias] = manifest.RegistryConfig{
			URL:  url,
			Auth: auth,
		}

		// Add another?
		fmt.Print("\nAdicionar outro registry? (y/N): ")
		another, _ := reader.ReadString('\n')
		another = strings.TrimSpace(strings.ToLower(another))
		if another != "y" && another != "yes" {
			break
		}
		fmt.Println()
	}

	if err := manifest.Save(".", m); err != nil {
		return fmt.Errorf("saving manifest: %w", err)
	}

	fmt.Printf("\namaru.json criado. Rode `amaru browse` para ver skills disponíveis.\n")
	return nil
}

func suggestAlias(url string) string {
	// For "github:org/repo-name", suggest the part after the last / without "-skills" suffix
	url = strings.TrimPrefix(url, "github:")
	url = strings.TrimPrefix(url, "https://github.com/")
	parts := strings.Split(url, "/")
	if len(parts) >= 2 {
		name := parts[len(parts)-1]
		name = strings.TrimSuffix(name, "-skills")
		name = strings.TrimSuffix(name, ".git")
		return name
	}
	return "default"
}
