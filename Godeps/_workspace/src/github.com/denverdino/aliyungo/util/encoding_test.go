package util

import (
	"testing"
	"time"
)

type SubStruct struct {
	A string
	B int
}

type TestStruct struct {
	Format      string
	Version     string
	AccessKeyId string
	Timestamp   time.Time
	Empty       string
	IntValue    int      `ArgName:"int-value"`
	BoolPtr     *bool    `ArgName:"bool-ptr"`
	IntPtr      *int     `ArgName:"int-ptr"`
	StringArray []string `ArgName:"str-array"`
	StructArray []SubStruct
}

func TestConvertToQueryValues(t *testing.T) {
	boolValue := true
	request := TestStruct{
		Format:      "JSON",
		Version:     "1.0",
		Timestamp:   time.Date(2015, time.Month(5), 26, 1, 2, 3, 4, time.UTC),
		IntValue:    10,
		BoolPtr:     &boolValue,
		StringArray: []string{"abc", "xyz"},
		StructArray: []SubStruct{
			SubStruct{A: "a", B: 1},
			SubStruct{A: "x", B: 2},
		},
	}
	result := ConvertToQueryValues(&request).Encode()
	const expectedResult = "Format=JSON&StructArray.1.A=a&StructArray.1.B=1&StructArray.2.A=x&StructArray.2.B=2&Timestamp=2015-05-26T01%3A02%3A03Z&Version=1.0&bool-ptr=true&int-value=10&str-array=%5B%22abc%22%2C%22xyz%22%5D"
	if result != expectedResult {
		t.Error("Incorrect encoding: ", result)
	}

}
