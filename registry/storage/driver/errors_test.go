package driver

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

func TestErrorFormat(t *testing.T) {
	drvName := "foo"
	errMsg := "unexpected error"

	e := Error{
		DriverName: drvName,
		Detail:     errors.New(errMsg),
	}

	exp := fmt.Sprintf("%s: %s", drvName, errMsg)

	if e.Error() != exp {
		t.Errorf("expected: %s, got: %s", exp, e.Error())
	}

	b, err := json.Marshal(&e)
	if err != nil {
		t.Fatal(err)
	}
	expJSON := `{"driver":"foo","detail":"unexpected error"}`
	if gotJSON := string(b); gotJSON != expJSON {
		t.Fatalf("expected JSON: %s,\n got: %s", expJSON, gotJSON)
	}
}

func TestErrors(t *testing.T) {
	t.Parallel()
	drvName := "foo"

	testCases := []struct {
		name    string
		errs    Errors
		exp     string
		expJSON string
	}{
		{
			name:    "no details",
			errs:    Errors{DriverName: drvName},
			exp:     fmt.Sprintf("%s: <nil>", drvName),
			expJSON: `{"driver":"foo","details":[]}`,
		},
		{
			name:    "single detail",
			errs:    Errors{DriverName: drvName, Errs: []error{errors.New("err msg")}},
			exp:     fmt.Sprintf("%s: err msg", drvName),
			expJSON: `{"driver":"foo","details":["err msg"]}`,
		},
		{
			name:    "multiple details",
			errs:    Errors{DriverName: drvName, Errs: []error{errors.New("err msg1"), errors.New("err msg2")}},
			exp:     fmt.Sprintf("%s: errors:\nerr msg1\nerr msg2\n", drvName),
			expJSON: `{"driver":"foo","details":["err msg1","err msg2"]}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.errs.Error(); got != tc.exp {
				t.Errorf("got error: %s, expected: %s", got, tc.exp)
			}
			b, err := json.Marshal(&tc.errs)
			if err != nil {
				t.Fatal(err)
			}
			if gotJSON := string(b); gotJSON != tc.expJSON {
				t.Errorf("expected JSON: %s,\n got: %s", tc.expJSON, gotJSON)
			}
		})
	}
}
