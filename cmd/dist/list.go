package main

import "github.com/codegangsta/cli"

var (
	commandList = cli.Command{
		Name:   "images",
		Usage:  "List available images",
		Action: imageList,
	}
)

func imageList(c *cli.Context) {
}
