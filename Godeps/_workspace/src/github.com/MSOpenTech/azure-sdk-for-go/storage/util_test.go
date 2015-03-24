package storage

import (
	"io/ioutil"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

func Test_timeRfc1123Formatted(t *testing.T) {
	now := time.Now().UTC()

	expectedLayout := "Mon, 02 Jan 2006 15:04:05 GMT"
	expected := now.Format(expectedLayout)

	if output := timeRfc1123Formatted(now); output != expected {
		t.Errorf("Expected: %s, got: %s", expected, output)
	}
}

func Test_mergeParams(t *testing.T) {
	v1 := url.Values{
		"k1": {"v1"},
		"k2": {"v2"}}
	v2 := url.Values{
		"k1": {"v11"},
		"k3": {"v3"}}

	out := mergeParams(v1, v2)
	if v := out.Get("k1"); v != "v1" {
		t.Errorf("Wrong value for k1: %s", v)
	}

	if v := out.Get("k2"); v != "v2" {
		t.Errorf("Wrong value for k2: %s", v)
	}

	if v := out.Get("k3"); v != "v3" {
		t.Errorf("Wrong value for k3: %s", v)
	}

	if v := out["k1"]; !reflect.DeepEqual(v, []string{"v1", "v11"}) {
		t.Errorf("Wrong multi-value for k1: %s", v)
	}
}

func Test_prepareBlockListRequest(t *testing.T) {
	empty := []Block{}
	expected := `<?xml version="1.0" encoding="utf-8"?><BlockList></BlockList>`
	if out := prepareBlockListRequest(empty); expected != out {
		t.Errorf("Wrong block list. Expected: '%s', got: '%s'", expected, out)
	}

	blocks := []Block{{"foo", BlockStatusLatest}, {"bar", BlockStatusUncommitted}}
	expected = `<?xml version="1.0" encoding="utf-8"?><BlockList><Latest>foo</Latest><Uncommitted>bar</Uncommitted></BlockList>`
	if out := prepareBlockListRequest(blocks); expected != out {
		t.Errorf("Wrong block list. Expected: '%s', got: '%s'", expected, out)
	}
}

func Test_xmlUnmarshal(t *testing.T) {
	xml := `<?xml version="1.0" encoding="utf-8"?>
	<Blob>
		<Name>myblob</Name>
	</Blob>`

	body := ioutil.NopCloser(strings.NewReader(xml))

	var blob Blob
	err := xmlUnmarshal(body, &blob)
	if err != nil {
		t.Fatal(err)
	}

	if blob.Name != "myblob" {
		t.Fatal("Got wrong value")
	}
}
