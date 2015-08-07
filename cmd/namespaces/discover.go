package main

import (
	"os"

	"github.com/codegangsta/cli"
	"github.com/docker/distribution/namespace"
)

var (
	commandDiscover = cli.Command{
		Name:   "discover",
		Usage:  `Discover the configuration for the specified namespace. The results are added to the namespace configuration unless otherwise specified.`,
		Action: discover,
	}
)

func discover(ctx *cli.Context) {
	args := []string(ctx.Args())
	entries, err := discoverer.Discover(args...)
	if err != nil {
		errorf("error discovering %v: %v", ctx.Args(), err)
	}
	defer write()

	namespaces, err = namespaces.Join(entries)
	if err != nil {
		errorf("error adding discovered namespaces: %v", err)
	}

	namespace.WriteEntries(os.Stdout, entries)
}

type hardCodedDiscoverer int

func (hardCodedDiscoverer) Discover(namespaces ...string) (*namespace.Entries, error) {
	nsEntries := namespace.NewEntries()
	for _, ns := range namespaces {
		var entries [][]string
		switch ns {
		case "mycompany.com":
			entries = [][]string{
				{
					"mycompany.com/",
					"push",
					"https://registry.mycompany.com", "v2",
				},
				{
					"mycompany.com/",
					"pull",
					"https://registry.mycompany.com", "v2",
				},
				{
					"mycompany.com/production/",
					"push",
					"https://production.mycompany.com", "v2",
				},
			}
		case "redhat.com":
			entries = [][]string{
				{
					"redhat.com/",
					"push",
					"https://registry.docker.com", "v2",
				},
				{
					"redhat.com/",
					"pull",
					"https://registry.docker.com", "v2",
				},
			}
		case "docker.com":
			entries = [][]string{
				{
					"docker.com/",
					"push",
					"https://registry.docker.com", "v2",
				},
				{
					"docker.com/",
					"pull",
					"https://registry.docker.com",
				},
				{
					"docker.com/",
					"pull",
					"https://mirror0.docker.com", "mirror",
				},
				{
					"docker.com/",
					"pull",
					"https://mirror1.docker.com", "mirror",
				},
				{
					"docker.com/",
					"pull",
					"https://mirror2.docker.com", "mirror",
				},
			}
		}
		for _, entry := range entries {
			e, err := namespace.NewEntry(entry[0], entry[1], entry[2:]...)
			if err != nil {
				panic(err)
			}
			if err := nsEntries.Add(e); err != nil {
				panic(err)
			}

		}
	}

	return nsEntries, nil
}
