package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/barelias/amaru/internal/checker"
	"github.com/barelias/amaru/internal/ui"

	"github.com/spf13/cobra"
)

var checkQuiet bool

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Verifica atualizações disponíveis e drift local",
	Long:  "Compara lock local com registries. Reporta atualizações disponíveis e drift local.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCheck(cmd.Context())
	},
}

func init() {
	checkCmd.Flags().BoolVar(&checkQuiet, "quiet", false, "Output mínimo, só warnings")
	rootCmd.AddCommand(checkCmd)
}

func runCheck(ctx context.Context) error {
	m, err := loadManifest()
	if err != nil {
		return err
	}

	lock, err := loadLock()
	if err != nil {
		return err
	}

	// Check cache first
	if checkQuiet {
		if cached := checker.LoadCache("."); cached != nil {
			printCheckResult(cached, true)
			return nil
		}
	}

	clients, err := buildClients(ctx, m, checkQuiet)
	if err != nil {
		return err
	}

	if !checkQuiet {
		fmt.Println()
		for alias, regConf := range m.Registries {
			fmt.Printf("Checking %s (%s)...\n", alias, regConf.URL)
		}
	}

	result, err := checker.Check(ctx, ".", m, lock, clients)
	if err != nil {
		return fmt.Errorf("check failed: %w", err)
	}

	// Save to cache
	checker.SaveCache(".", result)

	printCheckResult(result, checkQuiet)
	return nil
}

func printCheckResult(result *checker.CheckResult, quiet bool) {
	if quiet {
		// Box format for session start
		if len(result.Updates) == 0 && len(result.Drifts) == 0 {
			return
		}
		var lines []string
		lines = append(lines, fmt.Sprintf("🐍 amaru: %d atualização(ões) disponível(is)", len(result.Updates)))
		for _, u := range result.Updates {
			suffix := ""
			if u.Category == "major" {
				suffix = " (MAJOR)"
			}
			lines = append(lines, fmt.Sprintf("  %s %s → %s%s [%s]", u.Name, u.Current, u.Latest, suffix, u.Registry))
		}
		for _, d := range result.Drifts {
			lines = append(lines, fmt.Sprintf("  %s: drift detectado [%s]", d.Name, d.Registry))
		}
		lines = append(lines, "")
		lines = append(lines, "  Rode `amaru update` para atualizar")
		ui.Box(lines)
		return
	}

	// Full output
	if len(result.Updates) > 0 {
		ui.Header("⚠ Atualizações disponíveis:")
		for _, u := range result.Updates {
			category := u.Category
			if category == "major" {
				category = strings.ToUpper(category) + " — breaking"
			}
			fmt.Printf("  %s: %s → %s (%s) [%s]\n", u.Name, u.Current, u.Latest, category, u.Registry)
			if u.LatestInRange != "" && u.LatestInRange != u.Latest {
				fmt.Printf("    (latest within range: %s)\n", u.LatestInRange)
			}
		}
	}

	if len(result.Drifts) > 0 {
		ui.Header("⚠ Drift detectado (editado localmente):")
		for _, d := range result.Drifts {
			fmt.Printf("  %s: hash local %s ≠ central %s (v%s) [%s]\n",
				d.Name, d.LocalHash, d.RemoteHash, d.Version, d.Registry)
		}
	}

	if len(result.Updates) == 0 && len(result.Drifts) == 0 {
		fmt.Println()
	}

	fmt.Printf("\n✓ %d skills/commands atualizados\n", result.UpToDate)
}
