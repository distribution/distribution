package main

import (
	"fmt"
	"log"
	"os"

	"github.com/codegangsta/cli"
	"github.com/docker/distribution/namespace"
)

// Important considerations:
//  1. Why are aliases and namespace config in the same file? It's important
//     to understand the mapping of namespaces to remotes. To keep this
//     simple, the mapping to a remote is managed within the namespace file.
//  2. Let's say we have the default config with foo.com trust added. We
//     should still resolve the aliases to pick up extra config, even if we
//     have a match:
//
//       $ ./namespaces list foo.com
//       docker.com/    pull     https://registry.docker.com
//       docker.com/    push     https://registry.docker.com
//       foo.com/       trust    http://foo.com/ca
//       *              alias    docker.com/library/
//
//     This is actually a bug.
//  3. Let's take the case of where we want to push to production. Does that
//     need a namespace? No.

// Broken cases:
//  1. 	./namespaces add local/* alias redhat.com/
//         -> doesn't get added

func init() {
	addDefault("docker.com", "pull", "https://registry.docker.com")
	addDefault("docker.com", "push", "https://registry.docker.com")
}

func main() {
	if err := read(); err != nil {
		log.Fatalf("error reading configuration: %v", err)
	}
	defer write()

	app := cli.NewApp()
	app.Name = "dist"
	app.Usage = "dist tool demo"

	app.Commands = []cli.Command{
		commandAdd,
		commandDiscover,
		commandList,
		commandRemove,
	}

	app.RunAndExitOnError()
}

// silly globals for now.
var discoverer hardCodedDiscoverer

var namespaces = namespace.NewEntries()
var defaults = namespace.NewEntries()

func addDefault(scope, action string, args ...string) {
	entry, err := namespace.NewEntry(scope, action, args...)
	if err != nil {
		panic(err)
	}
	if err := defaults.Add(entry); err != nil {
		panic(err)
	}
}

func read() error {
	fp, err := os.Open(".namespace.cfg")
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}

		log.Println("using defaults")
		namespaces = defaults

		return nil
	}
	defer fp.Close()

	parsed, err := namespace.ParseEntries(fp)
	if err != nil {
		return err
	}

	namespaces = parsed

	return nil
}

func write() error {
	fp, err := os.OpenFile(".namespace.cfg", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer fp.Close()

	return namespace.WriteEntries(fp, namespaces)
}

func errorf(format string, args ...interface{}) {
	fmt.Printf("* fatal: "+format+"\n", args...)
	os.Exit(1)
}
