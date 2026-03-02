package ui

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/fatih/color"
)

func init() {
	// Disable color output in tests for predictable assertions
	color.NoColor = true
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe error: %v", err)
	}
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("reading pipe: %v", err)
	}
	return string(out)
}

func TestBox(t *testing.T) {
	output := captureStdout(t, func() {
		Box([]string{"Hello", "World!"})
	})

	if !strings.Contains(output, "Hello") {
		t.Error("expected Hello in output")
	}
	if !strings.Contains(output, "World!") {
		t.Error("expected World! in output")
	}
	if !strings.Contains(output, "╭") {
		t.Error("expected top border")
	}
	if !strings.Contains(output, "╰") {
		t.Error("expected bottom border")
	}
}

func TestBoxEmpty(t *testing.T) {
	output := captureStdout(t, func() {
		Box([]string{})
	})

	if output != "" {
		t.Errorf("expected empty output for empty box, got %q", output)
	}
}

func TestTable(t *testing.T) {
	output := captureStdout(t, func() {
		Table([][]string{
			{"Name", "Version", "Status"},
			{"research", "1.0.0", "installed"},
			{"plan", "2.0.0", "pending"},
		})
	})

	if !strings.Contains(output, "Name") {
		t.Error("expected Name in output")
	}
	if !strings.Contains(output, "research") {
		t.Error("expected research in output")
	}
	if !strings.Contains(output, "plan") {
		t.Error("expected plan in output")
	}
}

func TestTableEmpty(t *testing.T) {
	output := captureStdout(t, func() {
		Table([][]string{})
	})

	if output != "" {
		t.Errorf("expected empty output for empty table, got %q", output)
	}
}

func TestCheck(t *testing.T) {
	output := captureStdout(t, func() {
		Check("installed %s", "research")
	})

	if !strings.Contains(output, "installed research") {
		t.Errorf("expected 'installed research', got %q", output)
	}
}

func TestWarn(t *testing.T) {
	output := captureStdout(t, func() {
		Warn("outdated %s", "plan")
	})

	if !strings.Contains(output, "outdated plan") {
		t.Errorf("expected 'outdated plan', got %q", output)
	}
}

func TestErr(t *testing.T) {
	output := captureStdout(t, func() {
		Err("failed %s", "install")
	})

	if !strings.Contains(output, "failed install") {
		t.Errorf("expected 'failed install', got %q", output)
	}
}
