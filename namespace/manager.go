package namespace

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// WriteEntries writes the contents of manager to the writer. The format is
// acceptable for configuration files and command line output.
func WriteEntries(wr io.Writer, es *Entries) error {
	tw := tabwriter.NewWriter(wr, 8, 8, 4, ' ', 0)
	defer tw.Flush()

	all, err := es.Find()
	if err != nil {
		return err
	}

	for _, entry := range all.entries {
		args := strings.Join(entry.args, "\t")
		fmt.Fprintf(tw, "%s\t%s\t%s\n", entry.scope, entry.action, args)
	}

	return nil
}

func parseEntry(s string) (Entry, error) {
	fields := strings.Fields(s)

	if len(fields) < 2 {
		return Entry{}, fmt.Errorf("entry invalid: must have <scope> <action> <args...>, %q", s)
	}

	return NewEntry(fields[0], fields[1], fields[2:]...)
}

// ParseEntries parses the serialized entries from a reader.
func ParseEntries(rd io.Reader) (*Entries, error) {
	scanner := bufio.NewScanner(rd)

	es := NewEntries()
	var line int
	for scanner.Scan() {
		line++
		value := strings.TrimSpace(scanner.Text())

		if value == "" || value[0] == '#' {
			// TODO(stevvooe): We should probably save comment lines, if
			// possible. They would have to be part of the following entry.
			continue // skip line
		}

		entry, err := parseEntry(value)
		if err != nil {
			return nil, fmt.Errorf("configuration invalid, line %d: %v", line, err)
		}

		es.Add(entry)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return es, nil
}
