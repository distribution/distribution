package namespace

import "testing"

func TestScope(t *testing.T) {
	scope, err := parseScope("name/scope")
	if err != nil {
		t.Fatalf("Error parsing scope: %s", err)
	}

	if !scope.Contains("name/scope") {
		t.Fatal("Expected scope to contain whole match")
	}

	if !scope.Contains("name/scope/sub") {
		t.Fatal("Expected scope to contain child match")
	}

	if !scope.Contains("name/scope/") {
		t.Fatal("Expected scope to contain whole with trailing slash")
	}

	if !scope.Contains("name/scope/sub/child") {
		t.Fatal("Expected scope to contain child match")
	}

	if scope.Contains("name") {
		t.Fatal("Expected scope to contain NOT contain ancestor")
	}

	if _, err := parseScope(""); err == nil {
		t.Fatal("Expected error parsing empty scope")
	}
}
