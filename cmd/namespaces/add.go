package main

import (
	"fmt"
	"os"

	"github.com/codegangsta/cli"
	"github.com/docker/distribution/namespace"
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

	var extra []string
	if len(args) > 2 {
		extra = args[2:]
	}
	entry, err := namespace.NewEntry(args[0], args[1], extra...)
	if err != nil {
		errorf("cannot add entry: %v", err)
	}

	namespaces.Add(entry)
}
