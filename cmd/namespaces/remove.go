package main

import (
	"github.com/codegangsta/cli"
	"github.com/docker/distribution/namespace"
)

var (
	commandRemove = cli.Command{
		Name:        "remove",
		Usage:       "remove an entry from the namespace configuration.",
		Description: "remove an entry from the namespace configuration.",
		Action:      remove,
	}
)

func remove(ctx *cli.Context) {
	defer write()
	args := []string(ctx.Args())

	if len(args) < 2 {
		cli.ShowCommandHelp(ctx, ctx.Command.Name)
		errorf("must specify a scope and action")
	}

	var extra []string
	if len(args) > 2 {
		extra = args[2:]
	}
	entry, err := namespace.NewEntry(args[0], args[1], extra...)
	if err != nil {
		errorf("error parsing entry: %v", err)
	}

	namespaces.Remove(entry)
}
