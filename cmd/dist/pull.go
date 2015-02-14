package main

import "github.com/codegangsta/cli"

var (
	commandPull = cli.Command{
		Name:   "pull",
		Usage:  "Pull and verify an image from a registry",
		Action: imagePull,
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "r,registry",
				Value: "hub.docker.io",
				Usage: "Registry to use (e.g.: localhost:5000)",
			},
		},
	}
)

func imagePull(c *cli.Context) {
}
