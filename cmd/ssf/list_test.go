package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestListPlain(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go missing")
	}
	bin := buildBinary(t)
	xdg := t.TempDir()
	a := t.TempDir()
	b := t.TempDir()
	runBin(t, bin, xdg, a)
	runBin(t, bin, xdg, b)

	cmd := exec.Command(bin, "list")
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+xdg, "HOME="+xdg)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("list: %v %s", err, out)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), out)
	}
}

func TestListJSON(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go missing")
	}
	bin := buildBinary(t)
	xdg := t.TempDir()
	a := t.TempDir()
	runBin(t, bin, xdg, a)

	cmd := exec.Command(bin, "list", "--json")
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+xdg, "HOME="+xdg)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("list --json: %v %s", err, out)
	}
	var rows []map[string]any
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if _, ok := rows[0]["path"]; !ok {
		t.Fatalf("missing path in row: %v", rows[0])
	}
}

func TestListEmpty(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go missing")
	}
	bin := buildBinary(t)
	xdg := t.TempDir()
	cmd := exec.Command(bin, "list")
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+xdg, "HOME="+xdg)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("list empty: %v %s", err, out)
	}
	if strings.TrimSpace(string(out)) != "" {
		t.Fatalf("expected empty output, got %q", out)
	}
}
