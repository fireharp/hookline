package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/fireharp/hookline/internal/recipes"
)

func recipeCommand(args []string, stdout io.Writer, registry recipes.Registry) error {
	if len(args) == 0 {
		return errors.New("usage: hookline recipe list [--json]")
	}
	switch args[0] {
	case "list":
		jsonOut := hasFlag(args, "--json")
		list := registry.List()
		if jsonOut {
			return writeJSON(stdout, list)
		}
		for _, item := range list {
			status := "available"
			if item.Enabled {
				status = "enabled"
			}
			fmt.Fprintf(stdout, "%s\t%s\t%s\n", item.ID, status, item.Title)
		}
		return nil
	default:
		return errors.New("usage: hookline recipe list [--json]")
	}
}
