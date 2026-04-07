package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/fclairamb/ssf/internal/config"
	"github.com/fclairamb/ssf/internal/registry"
)

func listCommand() *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "print registered directories",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "json",
				Usage: "emit a JSON array",
			},
		},
		Action: runList,
	}
}

func runList(ctx context.Context, cmd *cli.Command) error {
	cfgPath, err := config.DefaultPath()
	if err != nil {
		return err
	}
	reg, err := registry.New(cfgPath)
	if err != nil {
		return err
	}
	entries, err := buildEntries(reg)
	if err != nil {
		return err
	}

	if cmd.Bool("json") {
		type row struct {
			Path       string `json:"path"`
			Display    string `json:"display"`
			Kind       string `json:"kind"`
			LastOpened string `json:"last_opened"`
		}
		rows := make([]row, 0, len(entries))
		for _, e := range entries {
			rows = append(rows, row{
				Path:       e.Path,
				Display:    e.Display,
				Kind:       string(e.Kind),
				LastOpened: e.LastOpen.UTC().Format("2006-01-02T15:04:05Z"),
			})
		}
		return json.NewEncoder(os.Stdout).Encode(rows)
	}

	for _, e := range entries {
		fmt.Fprintln(os.Stdout, e.Display)
	}
	return nil
}
