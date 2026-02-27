package ui

import (
	"fmt"
	"strings"

	"github.com/fatih/color"
)

var (
	Success = color.New(color.FgGreen).SprintFunc()
	Warning = color.New(color.FgYellow).SprintFunc()
	Error   = color.New(color.FgRed).SprintFunc()
	Bold    = color.New(color.Bold).SprintFunc()
	Dim     = color.New(color.Faint).SprintFunc()
)

// Check prints a green checkmark with a message.
func Check(format string, args ...interface{}) {
	fmt.Printf("  %s %s\n", Success("✓"), fmt.Sprintf(format, args...))
}

// Warn prints a yellow warning with a message.
func Warn(format string, args ...interface{}) {
	fmt.Printf("  %s %s\n", Warning("⚠"), fmt.Sprintf(format, args...))
}

// Err prints a red error with a message.
func Err(format string, args ...interface{}) {
	fmt.Printf("  %s %s\n", Error("✗"), fmt.Sprintf(format, args...))
}

// Header prints a section header.
func Header(format string, args ...interface{}) {
	fmt.Printf("\n%s\n", Bold(fmt.Sprintf(format, args...)))
}

// Box prints a message in a box (for session start warnings).
func Box(lines []string) {
	if len(lines) == 0 {
		return
	}

	maxLen := 0
	for _, line := range lines {
		if len(line) > maxLen {
			maxLen = len(line)
		}
	}
	width := maxLen + 4

	top := "╭" + strings.Repeat("─", width) + "╮"
	bottom := "╰" + strings.Repeat("─", width) + "╯"

	fmt.Println(top)
	for _, line := range lines {
		padding := strings.Repeat(" ", maxLen-len(line))
		fmt.Printf("│  %s%s  │\n", line, padding)
	}
	fmt.Println(bottom)
}

// Table prints a simple aligned table.
func Table(rows [][]string) {
	if len(rows) == 0 {
		return
	}

	// Calculate column widths
	colWidths := make([]int, len(rows[0]))
	for _, row := range rows {
		for i, cell := range row {
			if i < len(colWidths) && len(cell) > colWidths[i] {
				colWidths[i] = len(cell)
			}
		}
	}

	for _, row := range rows {
		for i, cell := range row {
			if i < len(colWidths) {
				fmt.Printf("  %-*s", colWidths[i]+2, cell)
			}
		}
		fmt.Println()
	}
}
