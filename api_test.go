package registry

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"testing"

	"github.com/Sirupsen/logrus"
	_ "github.com/docker/docker-registry/storagedriver/inmemory"

	"github.com/gorilla/handlers"

	"github.com/docker/docker-registry/common/testutil"
	"github.com/docker/docker-registry/configuration"
	"github.com/docker/docker-registry/digest"
)

// TestLayerAPI conducts a full of the of the layer api.
func TestLayerAPI(t *testing.T) {
	// TODO(stevvooe): This test code is complete junk but it should cover the
	// complete flow. This must be broken down and checked against the
	// specification *before* we submit the final to docker core.

	config := configuration.Configuration{
		Storage: configuration.Storage{
			"inmemory": configuration.Parameters{},
		},
	}

	app := NewApp(config)
	server := httptest.NewServer(handlers.CombinedLoggingHandler(os.Stderr, app))
	router := v2APIRouter()

	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("error parsing server url: %v", err)
	}

	imageName := "foo/bar"
	// "build" our layer file
	layerFile, tarSumStr, err := testutil.CreateRandomTarFile()
	if err != nil {
		t.Fatalf("error creating random layer file: %v", err)
	}

	layerDigest := digest.Digest(tarSumStr)

	// -----------------------------------
	// Test fetch for non-existent content
	r, err := router.GetRoute(routeNameBlob).Host(u.Host).
		URL("name", imageName,
		"digest", tarSumStr)

	resp, err := http.Get(r.String())
	if err != nil {
		t.Fatalf("unexpected error fetching non-existent layer: %v", err)
	}

	switch resp.StatusCode {
	case http.StatusNotFound:
		break // expected
	default:
		d, err := httputil.DumpResponse(resp, true)
		if err != nil {
			t.Fatalf("unexpected status fetching non-existent layer: %v, %v", resp.StatusCode, resp.Status)
		}

		t.Logf("response:\n%s", string(d))
		t.Fatalf("unexpected status fetching non-existent layer: %v, %v", resp.StatusCode, resp.Status)
	}

	// ------------------------------------------
	// Test head request for non-existent content
	resp, err = http.Head(r.String())
	if err != nil {
		t.Fatalf("unexpected error checking head on non-existent layer: %v", err)
	}

	switch resp.StatusCode {
	case http.StatusNotFound:
		break // expected
	default:
		d, err := httputil.DumpResponse(resp, true)
		if err != nil {
			t.Fatalf("unexpected status checking head on non-existent layer: %v, %v", resp.StatusCode, resp.Status)
		}

		t.Logf("response:\n%s", string(d))
		t.Fatalf("unexpected status checking head on non-existent layer: %v, %v", resp.StatusCode, resp.Status)
	}

	// ------------------------------------------
	// Upload a layer
	r, err = router.GetRoute(routeNameBlobUpload).Host(u.Host).
		URL("name", imageName)
	if err != nil {
		t.Fatalf("error starting layer upload: %v", err)
	}

	resp, err = http.Post(r.String(), "", nil)
	if err != nil {
		t.Fatalf("error starting layer upload: %v", err)
	}

	if resp.StatusCode != http.StatusAccepted {
		d, err := httputil.DumpResponse(resp, true)
		if err != nil {
			t.Fatalf("unexpected status starting layer upload: %v, %v", resp.StatusCode, resp.Status)
		}

		t.Logf("response:\n%s", string(d))
		t.Fatalf("unexpected status starting layer upload: %v, %v", resp.StatusCode, resp.Status)
	}

	if resp.Header.Get("Location") == "" { // TODO(stevvooe): Need better check here.
		t.Fatalf("unexpected Location: %q != %q", resp.Header.Get("Location"), "foo")
	}

	if resp.Header.Get("Content-Length") != "0" {
		t.Fatalf("unexpected content-length: %q != %q", resp.Header.Get("Content-Length"), "0")
	}

	layerLength, _ := layerFile.Seek(0, os.SEEK_END)
	layerFile.Seek(0, os.SEEK_SET)

	uploadURLStr := resp.Header.Get("Location")

	// TODO(sday): Cancel the layer upload here and restart.

	query := url.Values{
		"digest": []string{layerDigest.String()},
		"length": []string{fmt.Sprint(layerLength)},
	}

	uploadURL, err := url.Parse(uploadURLStr)
	if err != nil {
		t.Fatalf("unexpected error parsing url: %v", err)
	}

	uploadURL.RawQuery = query.Encode()

	// Just do a monolithic upload
	req, err := http.NewRequest("PUT", uploadURL.String(), layerFile)
	if err != nil {
		t.Fatalf("unexpected error creating new request: %v", err)
	}

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error doing put: %v", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusCreated:
		break // expected
	default:
		d, err := httputil.DumpResponse(resp, true)
		if err != nil {
			t.Fatalf("unexpected status putting chunk: %v, %v", resp.StatusCode, resp.Status)
		}

		t.Logf("response:\n%s", string(d))
		t.Fatalf("unexpected status putting chunk: %v, %v", resp.StatusCode, resp.Status)
	}

	if resp.Header.Get("Location") == "" {
		t.Fatalf("unexpected Location: %q", resp.Header.Get("Location"))
	}

	if resp.Header.Get("Content-Length") != "0" {
		t.Fatalf("unexpected content-length: %q != %q", resp.Header.Get("Content-Length"), "0")
	}

	layerURL := resp.Header.Get("Location")

	// ------------------------
	// Use a head request to see if the layer exists.
	resp, err = http.Head(layerURL)
	if err != nil {
		t.Fatalf("unexpected error checking head on non-existent layer: %v", err)
	}

	switch resp.StatusCode {
	case http.StatusOK:
		break // expected
	default:
		d, err := httputil.DumpResponse(resp, true)
		if err != nil {
			t.Fatalf("unexpected status checking head on layer: %v, %v", resp.StatusCode, resp.Status)
		}

		t.Logf("response:\n%s", string(d))
		t.Fatalf("unexpected status checking head on layer: %v, %v", resp.StatusCode, resp.Status)
	}

	logrus.Infof("fetch the layer")
	// ----------------
	// Fetch the layer!
	resp, err = http.Get(layerURL)
	if err != nil {
		t.Fatalf("unexpected error fetching layer: %v", err)
	}

	switch resp.StatusCode {
	case http.StatusOK:
		break // expected
	default:
		d, err := httputil.DumpResponse(resp, true)
		if err != nil {
			t.Fatalf("unexpected status fetching layer: %v, %v", resp.StatusCode, resp.Status)
		}

		t.Logf("response:\n%s", string(d))
		t.Fatalf("unexpected status fetching layer: %v, %v", resp.StatusCode, resp.Status)
	}

	// Verify the body
	verifier := digest.NewDigestVerifier(layerDigest)
	io.Copy(verifier, resp.Body)

	if !verifier.Verified() {
		d, err := httputil.DumpResponse(resp, true)
		if err != nil {
			t.Fatalf("unexpected status checking head on layer ayo!: %v, %v", resp.StatusCode, resp.Status)
		}

		t.Logf("response:\n%s", string(d))
		t.Fatalf("response body did not pass verification")
	}
}
