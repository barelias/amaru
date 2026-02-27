package cmd

import (
	"fmt"

	"github.com/barelias/amaru/internal/manifest"

	"github.com/spf13/cobra"
)

var ignoreCmd = &cobra.Command{
	Use:   "ignore <name>",
	Short: "Marca item como drift aceito",
	Long:  "Marca uma skill/command como 'drift aceito' — não reporta warning de hash mismatch.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runIgnore(args[0])
	},
}

var unignoreCmd = &cobra.Command{
	Use:   "unignore <name>",
	Short: "Remove item da lista de drift aceito",
	Long:  "Remove uma skill/command da lista de ignored, voltando a reportar drift.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runUnignore(args[0])
	},
}

func init() {
	rootCmd.AddCommand(ignoreCmd)
	rootCmd.AddCommand(unignoreCmd)
}

func runIgnore(name string) error {
	m, err := loadManifest()
	if err != nil {
		return err
	}

	// Check if already ignored
	if m.IsIgnored(name) {
		return fmt.Errorf("%s já está na lista de ignored", name)
	}

	// Check if item exists in manifest
	_, inSkills := m.Skills[name]
	_, inCommands := m.Commands[name]
	if !inSkills && !inCommands {
		return fmt.Errorf("%s não encontrado no manifesto", name)
	}

	m.Ignored = append(m.Ignored, name)

	if err := manifest.Save(".", m); err != nil {
		return fmt.Errorf("saving manifest: %w", err)
	}

	fmt.Printf("%s marcado como drift aceito. Não será reportado no check.\n", name)
	fmt.Printf("Para reverter: amaru unignore %s\n", name)
	return nil
}

func runUnignore(name string) error {
	m, err := loadManifest()
	if err != nil {
		return err
	}

	if !m.IsIgnored(name) {
		return fmt.Errorf("%s não está na lista de ignored", name)
	}

	var newIgnored []string
	for _, ignored := range m.Ignored {
		if ignored != name {
			newIgnored = append(newIgnored, ignored)
		}
	}
	m.Ignored = newIgnored

	if err := manifest.Save(".", m); err != nil {
		return fmt.Errorf("saving manifest: %w", err)
	}

	fmt.Printf("%s removido da lista de ignored. Drift será reportado no check.\n", name)
	return nil
}
