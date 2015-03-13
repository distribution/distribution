package main

import (
	"strings"

	"github.com/codegangsta/cli"
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

	if len(ctx.Args()) < 2 {
		cli.ShowCommandHelp(ctx, ctx.Command.Name)
		errorf("must specify a scope and action")
	}

	entry, err := ParseEntry(strings.Join(ctx.Args(), " "))
	if err != nil {
		errorf("error parsing entry: %v", err)
	}

	namespaces.Remove(entry)
}
