package namespace

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
)

var (
	flagExp     = regexp.MustCompile("^([[:alnum:]][[:word:]]*)(?:=([[:graph:]]+))?$")
	priorityExp = regexp.MustCompile("^[[:digit:]]+$")
)

// RemoteEndpoint represents a remote server which serves
// an API for specific functions (i.e. push, pull, search, trust).
type RemoteEndpoint struct {
	// Action represents the functional API that this remote
	// endpoint is serving.  Their may be multiple actions
	// allowed for a single API, but an action should always
	// map to exactly one API. A subset of the API may be used
	// for specific actions.
	Action string

	// BaseURL represents the URL in which the API is served.
	// For HTTP APIs this may include a path which should be
	// considered the root for the API. The information needed
	// to interpret how this base URL is used may be derived
	// from the flags for this endpoint.
	BaseURL *url.URL

	// Priority represents the relative priority of this
	// endpoint over other endpoints with the same action.
	// This priority defaults to 0 and endpoints with a
	// higher priority should be considered better matches
	// over endpoints with a lower priority.
	Priority int

	// Flags holds action-specific flags for the endpoint
	// which include version information and specific
	// requirements for interacting with the endpoint.
	Flags map[string]string
}

// createEndpoint parses the entry into a RemoteEndpoint.
// The arguments are treated as <endpoint> [<priority>] [<flag>[=<value>]]...
func createEndpoint(entry Entry) (*RemoteEndpoint, error) {
	if len(entry.args) == 0 {
		return nil, errors.New("missing endpoint argument")
	}
	base, err := url.Parse(entry.args[0])
	if err != nil {
		return nil, err
	}
	// If scheme is empty, reparse with default scheme
	if base.Scheme == "" {
		base, err = url.Parse("https://" + entry.args[0])
		if err != nil {
			return nil, err
		}
	}

	priority := 0
	var prioritySet bool
	flags := map[string]string{}
	for _, arg := range entry.args[1:] {
		if !prioritySet {
			// Attempt match on
			if priorityExp.MatchString(arg) {
				i, err := strconv.Atoi(arg)
				if err != nil {
					return nil, err
				}
				priority = i
				prioritySet = true
				continue
			}
		}
		matches := flagExp.FindStringSubmatch(arg)
		if len(matches) == 2 {
			flags[matches[1]] = ""
		} else if len(matches) == 3 {
			flags[matches[1]] = matches[2]
		} else {
			return nil, fmt.Errorf("invalid flag %q", arg)
		}
	}

	return &RemoteEndpoint{
		Action:   entry.action,
		BaseURL:  base,
		Priority: priority,
		Flags:    flags,
	}, nil
}
