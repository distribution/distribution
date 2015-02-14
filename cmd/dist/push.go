package main

import "github.com/codegangsta/cli"

var (
	commandPush = cli.Command{
		Name:   "push",
		Usage:  "Push an image to a registry",
		Action: imagePush,
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "r,registry",
				Value: "hub.docker.io",
				Usage: "Registry to use (e.g.: localhost:5000)",
			},
		},
	}
)

func imagePush(*cli.Context) {
}
