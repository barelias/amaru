package cmd

import (
	"github.com/spf13/cobra"
)

const version = "0.1.0"

var rootCmd = &cobra.Command{
	Use:   "amaru",
	Short: "Gerenciador de skills e commands para Claude Code",
	Long: `amaru gerencia skills e commands para Claude Code via arquivo manifesto (amaru.json).
Suporta múltiplos registries (públicos e privados), verifica atualizações,
e emite warnings quando há versões novas disponíveis.`,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.Version = version
}
