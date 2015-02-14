package main

import (
	"os"

	"github.com/codegangsta/cli"
)

func main() {
	app := cli.NewApp()
	app.Name = "dist"
	app.Usage = "Package and ship Docker content"

	app.Action = commandList.Action
	app.Commands = []cli.Command{
		commandList,
		commandPull,
		commandPush,
	}
	app.Run(os.Args)
}
