package main

import (
	"os"

	"github.com/barelias/amaru/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
