package util

import (
	"encoding/json"
	"testing"
	"time"
)

func TestISO8601Time(t *testing.T) {
	now := NewISO6801Time(time.Now().UTC())

	data, err := json.Marshal(now)
	if err != nil {
		t.Fatal(err)
	}

	_, err = time.Parse(`"`+formatISO8601+`"`, string(data))
	if err != nil {
		t.Fatal(err)
	}

	var now2 ISO6801Time
	err = json.Unmarshal(data, &now2)
	if err != nil {
		t.Fatal(err)
	}

	if now != now2 {
		t.Errorf("Time %s does not equal expected %s", now2, now)
	}

	if now.String() != now2.String() {
		t.Fatalf("String format for %s does not equal expected %s", now2, now)
	}

	type TestTimeStruct struct {
		A int
		B *ISO6801Time
	}
	var testValue TestTimeStruct
	err = json.Unmarshal([]byte("{\"A\": 1, \"B\":\"\"}"), &testValue)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%v", testValue)
	if !testValue.B.IsDefault() {
		t.Fatal("Invaid Unmarshal result for ISO6801Time from empty value")
	}
	t.Logf("ISO6801Time String(): %s", now2.String())
}
