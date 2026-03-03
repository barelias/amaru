package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/barelias/amaru/internal/scaffold"
	"github.com/barelias/amaru/internal/ui"

	"github.com/spf13/cobra"
)

var repoCmd = &cobra.Command{
	Use:   "repo",
	Short: "Gerencia repositórios de registry",
	Long:  "Comandos para criar e gerenciar repositórios de registry amaru.",
}

var repoInitProject string

var repoInitCmd = &cobra.Command{
	Use:   "init [directory]",
	Short: "Cria estrutura de um novo registry repo",
	Long:  "Cria a estrutura de diretórios para um repositório de registry amaru centralizado.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := "."
		if len(args) > 0 {
			dir = args[0]
		}
		return runRepoInit(dir)
	},
}

func init() {
	repoInitCmd.Flags().StringVar(&repoInitProject, "project", "", "Nome do projeto inicial para setup de context")
	repoCmd.AddCommand(repoInitCmd)
	rootCmd.AddCommand(repoCmd)
}

func runRepoInit(dir string) error {
	project := repoInitProject

	if project == "" {
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Initial project name (optional, press Enter to skip): ")
		input, _ := reader.ReadString('\n')
		project = strings.TrimSpace(input)
	}

	cfg := scaffold.RepoConfig{
		Dir:     dir,
		Project: project,
	}

	if err := scaffold.ScaffoldRepo(cfg); err != nil {
		return fmt.Errorf("scaffolding registry: %w", err)
	}

	ui.Check("Registry scaffolded at %s", dir)
	fmt.Println("  Created: amaru_registry.json, AGENTS.md, .amaru_registry/{skills,commands,agents,context}/")
	if project != "" {
		fmt.Printf("  Created: .amaru_registry/context/%s/ with brainstorms/, plans/, solutions/\n", project)
		fmt.Printf("  Created: .amaru_registry/.sparse-profiles/%s\n", project)
	}
	fmt.Println("\n  Next steps:")
	fmt.Println("    1. git init && git add . && git commit -m 'Initial registry'")
	fmt.Println("    2. Push to GitHub")
	fmt.Println("    3. In consuming projects: amaru init")

	return nil
}
