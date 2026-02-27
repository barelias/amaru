package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/barelias/amaru/internal/ui"

	"github.com/spf13/cobra"
)

var browseRegistry string

var browseCmd = &cobra.Command{
	Use:   "browse",
	Short: "Lista skills/commands disponíveis nos registries",
	Long:  "Lista tudo disponível nos registries configurados (discovery).",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runBrowse(cmd.Context())
	},
}

func init() {
	browseCmd.Flags().StringVar(&browseRegistry, "registry", "", "Filtrar por registry")
	rootCmd.AddCommand(browseCmd)
}

func runBrowse(ctx context.Context) error {
	m, err := loadManifest()
	if err != nil {
		return err
	}

	clients, err := buildClients(ctx, m, true)
	if err != nil {
		return err
	}

	for alias, regConf := range m.Registries {
		if browseRegistry != "" && alias != browseRegistry {
			continue
		}

		client, ok := clients[alias]
		if !ok {
			continue
		}

		idx, err := client.FetchIndex(ctx)
		if err != nil {
			ui.Err("Failed to fetch %s: %v", alias, err)
			continue
		}

		fmt.Printf("\n[%s] %s\n", ui.Bold(alias), regConf.URL)

		if len(idx.Skills) > 0 {
			fmt.Println("  Skills:")
			var rows [][]string
			for name, entry := range idx.Skills {
				tags := ""
				if len(entry.Tags) > 0 {
					tags = "[" + strings.Join(entry.Tags, ", ") + "]"
				}
				rows = append(rows, []string{"    " + name, entry.Latest, tags, entry.Description})
			}
			ui.Table(rows)
		}

		if len(idx.Commands) > 0 {
			fmt.Println("  Commands:")
			var rows [][]string
			for name, entry := range idx.Commands {
				tags := ""
				if len(entry.Tags) > 0 {
					tags = "[" + strings.Join(entry.Tags, ", ") + "]"
				}
				rows = append(rows, []string{"    " + name, entry.Latest, tags, entry.Description})
			}
			ui.Table(rows)
		}
	}

	return nil
}
