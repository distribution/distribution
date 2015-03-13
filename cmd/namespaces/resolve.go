package main

import (
	"fmt"

	"github.com/codegangsta/cli"
)

var (
	commandResolve = cli.Command{
		Name:        "resolve",
		Usage:       "Using the current namespace configuration, return the canonical version of the argument.",
		Description: "Show the canonical namespace based on the current configuration.",
		Action:      resolve,
	}
)

func resolve(ctx *cli.Context) {
	args := []string(ctx.Args())

	if len(args) < 1 {
		errorf("please specify at least one name")
	}

	for _, arg := range args {
		fmt.Println(namespaces.Resolve(arg))
	}
}
