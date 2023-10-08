package token

// actionSet is a special type of stringSet.
type actionSet struct {
	stringSet
}

func newActionSet(actions ...string) actionSet {
	return actionSet{newStringSet(actions...)}
}

// Contains calls StringSet.Contains() for
// either "*" or the given action string.
func (s actionSet) contains(action string) bool {
	return s.stringSet.contains("*") || s.stringSet.contains(action)
}

// contains returns true if q is found in ss.
func contains(ss []string, q string) bool {
	for _, s := range ss {
		if s == q {
			return true
		}
	}

	return false
}

// containsAny returns true if any of q is found in ss.
func containsAny(ss []string, q []string) bool {
	for _, s := range ss {
		if contains(q, s) {
			return true
		}
	}

	return false
}
