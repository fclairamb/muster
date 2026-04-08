package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/urfave/cli/v3"
	"golang.org/x/term"

	"github.com/fclairamb/muster/internal/files"
)

// filesCommand returns the hidden `muster files <dir>` subcommand. It's spawned
// by the tmux right pane and runs forever until cancelled.
func filesCommand() *cli.Command {
	return &cli.Command{
		Name:      "files",
		Hidden:    true,
		Usage:     "internal: live file panel for the right tmux pane",
		ArgsUsage: "<dir>",
		Action:    runFiles,
	}
}

func runFiles(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 1 {
		return fmt.Errorf("usage: muster files <dir>")
	}
	dir := cmd.Args().First()

	// Honor Ctrl+C cleanly.
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	ticks := files.Tick(ctx, dir)
	// Hide cursor for the duration; restore on exit.
	fmt.Print("\x1b[?25l")
	defer fmt.Print("\x1b[?25h")
	for {
		select {
		case <-ctx.Done():
			return nil
		case _, ok := <-ticks:
			if !ok {
				return nil
			}
			width := termWidth()
			var buf bytes.Buffer
			files.Render(&buf, dir, width)
			// Home cursor, then write each line with clear-to-EOL so we
			// overwrite in place instead of clearing the whole screen
			// (which causes flicker). Trailing rows are wiped with \x1b[J.
			var out bytes.Buffer
			out.WriteString("\x1b[H")
			lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
			for _, line := range lines {
				out.WriteString(line)
				out.WriteString("\x1b[K\r\n")
			}
			out.WriteString("\x1b[J")
			os.Stdout.Write(out.Bytes())
		}
	}
}

// termWidth returns the current terminal width, falling back to 40 columns
// for the right pane (typical 30% of a 130-col screen).
func termWidth() int {
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		return w
	}
	return 40
}
