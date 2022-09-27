package token

import (
	"encoding/json"
	"testing"
)

func TestWeakStringList_Unmarshal(t *testing.T) {
	t.Run("OK", func(t *testing.T) {
		testCases := []struct {
			value    string
			expected WeakStringList
		}{
			{
				value:    `"audience"`,
				expected: WeakStringList{"audience"},
			},
			{
				value:    `["audience1", "audience2"]`,
				expected: WeakStringList{"audience1", "audience2"},
			},
			{
				value:    `null`,
				expected: nil,
			},
		}

		for _, testCase := range testCases {
			testCase := testCase

			t.Run("", func(t *testing.T) {
				var actual WeakStringList

				err := json.Unmarshal([]byte(testCase.value), &actual)
				if err != nil {
					t.Fatal(err)
				}

				assertStringListEqual(t, testCase.expected, actual)
			})
		}
	})

	t.Run("Error", func(t *testing.T) {
		var actual WeakStringList

		err := json.Unmarshal([]byte("1234"), &actual)
		if err == nil {
			t.Fatal("expected unmarshal to fail")
		}
	})
}

func TestWeakStringList_Marshal(t *testing.T) {
	value := WeakStringList{"audience"}

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

		return
	}
}
