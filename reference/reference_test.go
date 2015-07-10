package reference

/*
var refRegex = regexp.MustCompile(`^([a-z0-9]+(?:[-._][a-z0-9]+)*(?::[0-9]+(?:/[a-z0-9]+(?:[-._][a-z0-9]+)*)+|(?:/[a-z0-9]+(?:[-._][a-z0-9]+)*)+)?)(:[\w][\w.-]{0,127})?(@` + digest.DigestRegexp.String() + `)?$`)

func getRepo(s string) string {
	matches := refRegex.FindStringSubmatch(s)
	if len(matches) == 0 {
		return ""
	}
	return matches[1]
}

func testRepository(prefix string) error {
	for _, s := range []string{
		prefix + `@sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff`,
		prefix + `:frozen@sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff`,
		prefix + `:latest`,
		prefix,
	} {
		expected := getRepo(s)
		ref, err := Parse(s)
		if err != nil {
			if expected == "" {
				continue
			}
			return err
		}
		if repo := ref.Repository(); repo.String() != expected {
			return fmt.Errorf("repository string: expected %q, got: %q", expected, repo)
		}
		if refStr := ref.String(); refStr != s {
			return fmt.Errorf("reference string: expected %q, got: %q", s, refStr)
		}
	}
	return nil
}

func TestSimpleRepository(t *testing.T) {
	if err := testRepository(`busybox`); err != nil {
		t.Fatal(err)
	}
}

func TestUrlRepository(t *testing.T) {
	if err := testRepository(`docker.io/library/busybox`); err != nil {
		t.Fatal(err)
	}
}

func TestPort(t *testing.T) {
	if err := testRepository(`busybox:1234`); err != nil {
		t.Fatal(err)
	}
}
*/
