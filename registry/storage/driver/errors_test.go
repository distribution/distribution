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
	expJson := `{"driver":"foo","detail":"unexpected error"}`
	if gotJson := string(b); gotJson != expJson {
		t.Fatalf("expected JSON: %s,\n got: %s", expJson, gotJson)
	}
}

func TestErrors(t *testing.T) {
	t.Parallel()
	drvName := "foo"

	testCases := []struct {
		name    string
		errs    Errors
		exp     string
		expJson string
	}{
		{
			name:    "no details",
			errs:    Errors{DriverName: drvName},
			exp:     fmt.Sprintf("%s: <nil>", drvName),
			expJson: `{"driver":"foo","details":[]}`,
		},
		{
			name:    "single detail",
			errs:    Errors{DriverName: drvName, Errs: []error{errors.New("err msg")}},
			exp:     fmt.Sprintf("%s: err msg", drvName),
			expJson: `{"driver":"foo","details":["err msg"]}`,
		},
		{
			name:    "multiple details",
			errs:    Errors{DriverName: drvName, Errs: []error{errors.New("err msg1"), errors.New("err msg2")}},
			exp:     fmt.Sprintf("%s: errors:\nerr msg1\nerr msg2\n", drvName),
			expJson: `{"driver":"foo","details":["err msg1","err msg2"]}`,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.errs.Error(); got != tc.exp {
				t.Errorf("got error: %s, expected: %s", got, tc.exp)
			}
			b, err := json.Marshal(&tc.errs)
			if err != nil {
				t.Fatal(err)
			}
			if gotJson := string(b); gotJson != tc.expJson {
				t.Errorf("expected JSON: %s,\n got: %s", tc.expJson, gotJson)
			}
		})
	}
}
