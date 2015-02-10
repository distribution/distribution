package storage

import (
	"encoding/base64"
	"net/url"
	"testing"
)

func TestGetBaseUrl_Basic_Https(t *testing.T) {
	cli, err := NewBasicClient("foo", "YmFy")
	if err != nil {
		t.Fatal(err)
	}

	if cli.apiVersion != DefaultApiVersion {
		t.Fatalf("Wrong api version. Expected: '%s', got: '%s'", DefaultApiVersion, cli.apiVersion)
	}

	if err != nil {
		t.Fatal(err)
	}
	output := cli.getBaseUrl("table")

	if expected := "https://foo.table.core.windows.net"; output != expected {
		t.Fatalf("Wrong base url. Expected: '%s', got: '%s'", expected, output)
	}
}

func TestGetBaseUrl_Custom_NoHttps(t *testing.T) {
	apiVersion := DefaultApiVersion
	cli, err := NewClient("foo", "YmFy", "core.chinacloudapi.cn", apiVersion, false)
	if err != nil {
		t.Fatal(err)
	}

	if cli.apiVersion != apiVersion {
		t.Fatalf("Wrong api version. Expected: '%s', got: '%s'", apiVersion, cli.apiVersion)
	}

	output := cli.getBaseUrl("table")

	if expected := "http://foo.table.core.chinacloudapi.cn"; output != expected {
		t.Fatalf("Wrong base url. Expected: '%s', got: '%s'", expected, output)
	}
}

func TestGetEndpoint_None(t *testing.T) {
	cli, err := NewBasicClient("foo", "YmFy")
	if err != nil {
		t.Fatal(err)
	}
	output := cli.getEndpoint(blobServiceName, "", url.Values{})

	if expected := "https://foo.blob.core.windows.net/"; output != expected {
		t.Fatalf("Wrong endpoint url. Expected: '%s', got: '%s'", expected, output)
	}
}

func TestGetEndpoint_PathOnly(t *testing.T) {
	cli, err := NewBasicClient("foo", "YmFy")
	if err != nil {
		t.Fatal(err)
	}
	output := cli.getEndpoint(blobServiceName, "path", url.Values{})

	if expected := "https://foo.blob.core.windows.net/path"; output != expected {
		t.Fatalf("Wrong endpoint url. Expected: '%s', got: '%s'", expected, output)
	}
}

func TestGetEndpoint_ParamsOnly(t *testing.T) {
	cli, err := NewBasicClient("foo", "YmFy")
	if err != nil {
		t.Fatal(err)
	}
	params := url.Values{}
	params.Set("a", "b")
	params.Set("c", "d")
	output := cli.getEndpoint(blobServiceName, "", params)

	if expected := "https://foo.blob.core.windows.net/?a=b&c=d"; output != expected {
		t.Fatalf("Wrong endpoint url. Expected: '%s', got: '%s'", expected, output)
	}
}

func TestGetEndpoint_Mixed(t *testing.T) {
	cli, err := NewBasicClient("foo", "YmFy")
	if err != nil {
		t.Fatal(err)
	}
	params := url.Values{}
	params.Set("a", "b")
	params.Set("c", "d")
	output := cli.getEndpoint(blobServiceName, "path", params)

	if expected := "https://foo.blob.core.windows.net/path?a=b&c=d"; output != expected {
		t.Fatalf("Wrong endpoint url. Expected: '%s', got: '%s'", expected, output)
	}
}

func Test_getStandardHeaders(t *testing.T) {
	cli, err := NewBasicClient("foo", "YmFy")
	if err != nil {
		t.Fatal(err)
	}

	headers := cli.getStandardHeaders()
	if len(headers) != 2 {
		t.Fatal("Wrong standard header count")
	}
	if v, ok := headers["x-ms-version"]; !ok || v != cli.apiVersion {
		t.Fatal("Wrong version header")
	}
	if _, ok := headers["x-ms-date"]; !ok {
		t.Fatal("Missing date header")
	}
}

func Test_buildCanonicalizedResource(t *testing.T) {
	cli, err := NewBasicClient("foo", "YmFy")
	if err != nil {
		t.Fatal(err)
	}

	type test struct{ url, expected string }
	tests := []test{
		{"https://foo.blob.core.windows.net/path?a=b&c=d", "/foo/path\na:b\nc:d"},
		{"https://foo.blob.core.windows.net/?comp=list", "/foo/\ncomp:list"},
		{"https://foo.blob.core.windows.net/cnt/blob", "/foo/cnt/blob"},
	}

	for _, i := range tests {
		if out, err := cli.buildCanonicalizedResource(i.url); err != nil {
			t.Fatal(err)
		} else if out != i.expected {
			t.Fatalf("Wrong canonicalized resource. Expected:\n'%s', Got:\n'%s'", i.expected, out)
		}
	}
}

func Test_buildCanonicalizedHeader(t *testing.T) {
	cli, err := NewBasicClient("foo", "YmFy")
	if err != nil {
		t.Fatal(err)
	}

	type test struct {
		headers  map[string]string
		expected string
	}
	tests := []test{
		{map[string]string{}, ""},
		{map[string]string{"x-ms-foo": "bar"}, "x-ms-foo:bar"},
		{map[string]string{"foo:": "bar"}, ""},
		{map[string]string{"foo:": "bar", "x-ms-foo": "bar"}, "x-ms-foo:bar"},
		{map[string]string{
			"x-ms-version":   "9999-99-99",
			"x-ms-blob-type": "BlockBlob"}, "x-ms-blob-type:BlockBlob\nx-ms-version:9999-99-99"}}

	for _, i := range tests {
		if out := cli.buildCanonicalizedHeader(i.headers); out != i.expected {
			t.Fatalf("Wrong canonicalized resource. Expected:\n'%s', Got:\n'%s'", i.expected, out)
		}
	}
}

func TestReturnsStorageServiceError(t *testing.T) {
	cli, err := getBlobClient()
	if err != nil {
		t.Fatal(err)
	}

	// attempt to delete a nonexisting container
	_, err = cli.deleteContainer(randContainer())
	if err == nil {
		t.Fatal("Service has not returned an error")
	}

	if v, ok := err.(StorageServiceError); !ok {
		t.Fatal("Cannot assert to specific error")
	} else if v.StatusCode != 404 {
		t.Fatalf("Expected status:%d, got: %d", 404, v.StatusCode)
	} else if v.Code != "ContainerNotFound" {
		t.Fatalf("Expected code: %s, got: %s", "ContainerNotFound", v.Code)
	} else if v.RequestId == "" {
		t.Fatalf("RequestId does not exist")
	}
}

func Test_createAuthorizationHeader(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("bar"))
	cli, err := NewBasicClient("foo", key)
	if err != nil {
		t.Fatal(err)
	}

	canonicalizedString := `foobarzoo`
	expected := `SharedKey foo:h5U0ATVX6SpbFX1H6GNuxIMeXXCILLoIvhflPtuQZ30=`

	if out := cli.createAuthorizationHeader(canonicalizedString); out != expected {
		t.Fatalf("Wrong authorization header. Expected: '%s', Got:'%s'", expected, out)
	}
}
