package token

import (
	"encoding/json"
	"testing"
)

func TestAudienceList_Unmarshal(t *testing.T) {
	t.Parallel()
	t.Run("OK", func(t *testing.T) {
		tests := []struct {
			value    string
			expected AudienceList
		}{
			{
				value:    `"audience"`,
				expected: AudienceList{"audience"},
			},
			{
				value:    `["audience1", "audience2"]`,
				expected: AudienceList{"audience1", "audience2"},
			},
			{
				value:    `null`,
				expected: nil,
			},
		}

		for _, tc := range tests {
			tc := tc
			t.Run("", func(t *testing.T) {
				t.Parallel()
				var actual AudienceList

				err := json.Unmarshal([]byte(tc.value), &actual)
				if err != nil {
					t.Fatal(err)
				}

				assertStringListEqual(t, tc.expected, actual)
			})
		}
	})

	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		var actual AudienceList

		err := json.Unmarshal([]byte("1234"), &actual)
		if err == nil {
			t.Fatal("expected unmarshal to fail")
		}
	})
}

func TestAudienceList_Marshal(t *testing.T) {
	value := AudienceList{"audience"}

	expected := `["audience"]`

	actual, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}

	if expected != string(actual) {
		t.Errorf("expected marshaled list to be %v, got %v", expected, actual)
	}
}

func assertStringListEqual(t *testing.T, expected []string, actual []string) {
	t.Helper()

	if len(expected) != len(actual) {
		t.Errorf("length mismatch: expected %d long slice, got %d", len(expected), len(actual))

		return
	}

	for i, v := range expected {
		if v != actual[i] {
			t.Errorf("expected %d. item to be %q, got %q", i, v, actual[i])
		}
	}
}
