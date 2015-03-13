package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/codegangsta/cli"
)

var (
	commandAdd = cli.Command{
		Name:        "add",
		Usage:       "Add an entry to the namespace configuration.",
		Description: "Add an entry to the namespace configuration.",
		Action:      add,
	}
)

func add(ctx *cli.Context) {
	defer write()
	args := []string(ctx.Args())

	if len(args) < 2 {
		cli.ShowCommandHelp(ctx, ctx.Command.Name)
		fmt.Fprintln(os.Stderr, "must specify a scope and action")
		os.Exit(1)
	}

	entry, err := ParseEntry(strings.Join([]string(args), " "))
	if err != nil {
		errorf("cannot add entry: %v", err)
	}

	namespaces.Add(entry)
}
