package main

import (
	"os"
	"sort"

	"github.com/codegangsta/cli"
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

	if err := namespaces.Add(entries...); err != nil {
		errorf("error adding discovered namespaces: %v", err)
	}

	WriteManager(os.Stdout, &entries)
}

type hardCodedDiscoverer int

func (hardCodedDiscoverer) Discover(namespaces ...string) (Entries, error) {
	var entries Entries

	for _, namespace := range namespaces {
		switch namespace {
		case "mycompany.com":
			entries = append(entries, Entries{
				{
					Scope:  "mycompany.com/",
					Action: "push",
					Args:   []string{"https://registry.mycompany.com", "v2"},
				},
				{
					Scope:  "mycompany.com/",
					Action: "pull",
					Args:   []string{"https://registry.mycompany.com", "v2"},
				},
				{
					Scope:  "mycompany.com/production/",
					Action: "push",
					Args:   []string{"https://production.mycompany.com", "v2"},
				},
				{
					Scope:  "production/",
					Action: "alias",
					// Note the lack of ending slash here: This means that
					// only a replacement should be used.
					Args: []string{"mycompany.com/production"},
				},
				{
					Scope:  "staging/",
					Action: "alias",
					// Note the lack of ending slash here: This means that
					// only a replacement should be used.
					Args: []string{"mycompany.com"},
				},
			}...)
		case "redhat.com":
			entries = append(entries, Entries{
				{
					Scope:  "redhat.com/",
					Action: "push",
					Args:   []string{"https://registry.docker.com", "v2"},
				},
				{
					Scope:  "redhat.com/",
					Action: "pull",
					Args:   []string{"https://registry.docker.com", "v2"},
				},
				{
					Scope:  "redhat/",
					Action: "alias",
					// Note the lack of ending slash here: This means that
					// only a replacement should be used.
					Args: []string{"redhat.com"},
				},
			}...)
		case "docker.com":
			entries = append(entries, Entries{
				{
					Scope:  "docker.com/",
					Action: "push",
					Args:   []string{"https://registry.docker.com", "v2"},
				},
				{
					Scope:  "docker.com/",
					Action: "pull",
					Args:   []string{"https://registry.docker.com"},
				},
				{
					Scope:  "docker.com/",
					Action: "pull",
					Args:   []string{"https://mirror0.docker.com", "mirror"},
				},
				{
					Scope:  "docker.com/",
					Action: "pull",
					Args:   []string{"https://mirror1.docker.com", "mirror"},
				},
				{
					Scope:  "docker.com/",
					Action: "pull",
					Args:   []string{"https://mirror2.docker.com", "mirror"},
				},
			}...)
		}
	}

	sort.Stable(entries)
	return entries, nil
}
