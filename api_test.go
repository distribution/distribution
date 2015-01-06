package registry

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"testing"

	"github.com/docker/distribution/api/v2"
	"github.com/docker/distribution/configuration"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest"
	_ "github.com/docker/distribution/storagedriver/inmemory"
	"github.com/docker/distribution/testutil"
	"github.com/docker/libtrust"
	"github.com/gorilla/handlers"
)

// TestCheckAPI hits the base endpoint (/v2/) ensures we return the specified
// 200 OK response.
func TestCheckAPI(t *testing.T) {
	config := configuration.Configuration{
		Storage: configuration.Storage{
			"inmemory": configuration.Parameters{},
		},
	}

	app := NewApp(config)
	server := httptest.NewServer(handlers.CombinedLoggingHandler(os.Stderr, app))
	builder, err := v2.NewURLBuilderFromString(server.URL)

	if err != nil {
		t.Fatalf("error creating url builder: %v", err)
	}

	baseURL, err := builder.BuildBaseURL()
	if err != nil {
		t.Fatalf("unexpected error building base url: %v", err)
	}

	resp, err := http.Get(baseURL)
	if err != nil {
		t.Fatalf("unexpected error issuing request: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "issuing api base check", resp, http.StatusOK)
	checkHeaders(t, resp, http.Header{
		"Content-Type":   []string{"application/json; charset=utf-8"},
		"Content-Length": []string{"2"},
	})

	p, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("unexpected error reading response body: %v", err)
	}

	if string(p) != "{}" {
		t.Fatalf("unexpected response body: %v", string(p))
	}
}

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
	builder, err := v2.NewURLBuilderFromString(server.URL)

	if err != nil {
		t.Fatalf("error creating url builder: %v", err)
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
	layerURL, err := builder.BuildBlobURL(imageName, layerDigest)
	if err != nil {
		t.Fatalf("error building url: %v", err)
	}

	resp, err := http.Get(layerURL)
	if err != nil {
		t.Fatalf("unexpected error fetching non-existent layer: %v", err)
	}

	checkResponse(t, "fetching non-existent content", resp, http.StatusNotFound)

	// ------------------------------------------
	// Test head request for non-existent content
	resp, err = http.Head(layerURL)
	if err != nil {
		t.Fatalf("unexpected error checking head on non-existent layer: %v", err)
	}

	checkResponse(t, "checking head on non-existent layer", resp, http.StatusNotFound)

	// ------------------------------------------
	// Upload a layer
	layerUploadURL, err := builder.BuildBlobUploadURL(imageName)
	if err != nil {
		t.Fatalf("error building upload url: %v", err)
	}

	resp, err = http.Post(layerUploadURL, "", nil)
	if err != nil {
		t.Fatalf("error starting layer upload: %v", err)
	}

	checkResponse(t, "starting layer upload", resp, http.StatusAccepted)
	checkHeaders(t, resp, http.Header{
		"Location":       []string{"*"},
		"Content-Length": []string{"0"},
	})

	layerLength, _ := layerFile.Seek(0, os.SEEK_END)
	layerFile.Seek(0, os.SEEK_SET)

	// TODO(sday): Cancel the layer upload here and restart.

	uploadURLBase := startPushLayer(t, builder, imageName)
	pushLayer(t, builder, imageName, layerDigest, uploadURLBase, layerFile)

	// ------------------------
	// Use a head request to see if the layer exists.
	resp, err = http.Head(layerURL)
	if err != nil {
		t.Fatalf("unexpected error checking head on existing layer: %v", err)
	}

	checkResponse(t, "checking head on existing layer", resp, http.StatusOK)
	checkHeaders(t, resp, http.Header{
		"Content-Length": []string{fmt.Sprint(layerLength)},
	})

	// ----------------
	// Fetch the layer!
	resp, err = http.Get(layerURL)
	if err != nil {
		t.Fatalf("unexpected error fetching layer: %v", err)
	}

	checkResponse(t, "fetching layer", resp, http.StatusOK)
	checkHeaders(t, resp, http.Header{
		"Content-Length": []string{fmt.Sprint(layerLength)},
	})

	// Verify the body
	verifier := digest.NewDigestVerifier(layerDigest)
	io.Copy(verifier, resp.Body)

	if !verifier.Verified() {
		t.Fatalf("response body did not pass verification")
	}

	// Missing tests:
	// 	- Upload the same tarsum file under and different repository and
	//       ensure the content remains uncorrupted.
}

func TestManifestAPI(t *testing.T) {
	pk, err := libtrust.GenerateECP256PrivateKey()
	if err != nil {
		t.Fatalf("unexpected error generating private key: %v", err)
	}

	config := configuration.Configuration{
		Storage: configuration.Storage{
			"inmemory": configuration.Parameters{},
		},
	}

	app := NewApp(config)
	server := httptest.NewServer(handlers.CombinedLoggingHandler(os.Stderr, app))
	builder, err := v2.NewURLBuilderFromString(server.URL)
	if err != nil {
		t.Fatalf("unexpected error creating url builder: %v", err)
	}

	imageName := "foo/bar"
	tag := "thetag"

	manifestURL, err := builder.BuildManifestURL(imageName, tag)
	if err != nil {
		t.Fatalf("unexpected error getting manifest url: %v", err)
	}

	// -----------------------------
	// Attempt to fetch the manifest
	resp, err := http.Get(manifestURL)
	if err != nil {
		t.Fatalf("unexpected error getting manifest: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "getting non-existent manifest", resp, http.StatusNotFound)

	// TODO(stevvooe): Shoot. The error setup is not working out. The content-
	// type headers are being set after writing the status code.
	// if resp.Header.Get("Content-Type") != "application/json; charset=utf-8" {
	// 	t.Fatalf("unexpected content type: %v != 'application/json'",
	// 		resp.Header.Get("Content-Type"))
	// }
	dec := json.NewDecoder(resp.Body)

	var respErrs v2.Errors
	if err := dec.Decode(&respErrs); err != nil {
		t.Fatalf("unexpected error decoding error response: %v", err)
	}

	if len(respErrs.Errors) == 0 {
		t.Fatalf("expected errors in response")
	}

	if respErrs.Errors[0].Code != v2.ErrorCodeManifestUnknown {
		t.Fatalf("expected manifest unknown error: got %v", respErrs)
	}

	tagsURL, err := builder.BuildTagsURL(imageName)
	if err != nil {
		t.Fatalf("unexpected error building tags url: %v", err)
	}

	resp, err = http.Get(tagsURL)
	if err != nil {
		t.Fatalf("unexpected error getting unknown tags: %v", err)
	}
	defer resp.Body.Close()

	// Check that we get an unknown repository error when asking for tags
	checkResponse(t, "getting unknown manifest tags", resp, http.StatusNotFound)
	dec = json.NewDecoder(resp.Body)
	if err := dec.Decode(&respErrs); err != nil {
		t.Fatalf("unexpected error decoding error response: %v", err)
	}

	if len(respErrs.Errors) == 0 {
		t.Fatalf("expected errors in response")
	}

	if respErrs.Errors[0].Code != v2.ErrorCodeNameUnknown {
		t.Fatalf("expected respository unknown error: got %v", respErrs)
	}

	// --------------------------------
	// Attempt to push unsigned manifest with missing layers
	unsignedManifest := &manifest.Manifest{
		Name: imageName,
		Tag:  tag,
		FSLayers: []manifest.FSLayer{
			{
				BlobSum: "asdf",
			},
			{
				BlobSum: "qwer",
			},
		},
	}

	resp = putManifest(t, "putting unsigned manifest", manifestURL, unsignedManifest)
	defer resp.Body.Close()
	checkResponse(t, "posting unsigned manifest", resp, http.StatusBadRequest)

	dec = json.NewDecoder(resp.Body)
	if err := dec.Decode(&respErrs); err != nil {
		t.Fatalf("unexpected error decoding error response: %v", err)
	}

	var unverified int
	var missingLayers int
	var invalidDigests int

	for _, err := range respErrs.Errors {
		switch err.Code {
		case v2.ErrorCodeManifestUnverified:
			unverified++
		case v2.ErrorCodeBlobUnknown:
			missingLayers++
		case v2.ErrorCodeDigestInvalid:
			// TODO(stevvooe): This error isn't quite descriptive enough --
			// the layer with an invalid digest isn't identified.
			invalidDigests++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}

	if unverified != 1 {
		t.Fatalf("should have received one unverified manifest error: %v", respErrs)
	}

	if missingLayers != 2 {
		t.Fatalf("should have received two missing layer errors: %v", respErrs)
	}

	if invalidDigests != 2 {
		t.Fatalf("should have received two invalid digest errors: %v", respErrs)
	}

	// TODO(stevvooe): Add a test case where we take a mostly valid registry,
	// tamper with the content and ensure that we get a unverified manifest
	// error.

	// Push 2 random layers
	expectedLayers := make(map[digest.Digest]io.ReadSeeker)

	for i := range unsignedManifest.FSLayers {
		rs, dgstStr, err := testutil.CreateRandomTarFile()

		if err != nil {
			t.Fatalf("error creating random layer %d: %v", i, err)
		}
		dgst := digest.Digest(dgstStr)

		expectedLayers[dgst] = rs
		unsignedManifest.FSLayers[i].BlobSum = dgst

		uploadURLBase := startPushLayer(t, builder, imageName)
		pushLayer(t, builder, imageName, dgst, uploadURLBase, rs)
	}

	// -------------------
	// Push the signed manifest with all layers pushed.
	signedManifest, err := manifest.Sign(unsignedManifest, pk)
	if err != nil {
		t.Fatalf("unexpected error signing manifest: %v", err)
	}

	resp = putManifest(t, "putting signed manifest", manifestURL, signedManifest)

	checkResponse(t, "putting signed manifest", resp, http.StatusOK)

	resp, err = http.Get(manifestURL)
	if err != nil {
		t.Fatalf("unexpected error fetching manifest: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "fetching uploaded manifest", resp, http.StatusOK)

	var fetchedManifest manifest.SignedManifest
	dec = json.NewDecoder(resp.Body)
	if err := dec.Decode(&fetchedManifest); err != nil {
		t.Fatalf("error decoding fetched manifest: %v", err)
	}

	if !bytes.Equal(fetchedManifest.Raw, signedManifest.Raw) {
		t.Fatalf("manifests do not match")
	}

	// Ensure that the tag is listed.
	resp, err = http.Get(tagsURL)
	if err != nil {
		t.Fatalf("unexpected error getting unknown tags: %v", err)
	}
	defer resp.Body.Close()

	// Check that we get an unknown repository error when asking for tags
	checkResponse(t, "getting unknown manifest tags", resp, http.StatusOK)
	dec = json.NewDecoder(resp.Body)

	var tagsResponse tagsAPIResponse

	if err := dec.Decode(&tagsResponse); err != nil {
		t.Fatalf("unexpected error decoding error response: %v", err)
	}

	if tagsResponse.Name != imageName {
		t.Fatalf("tags name should match image name: %v != %v", tagsResponse.Name, imageName)
	}

	if len(tagsResponse.Tags) != 1 {
		t.Fatalf("expected some tags in response: %v", tagsResponse.Tags)
	}

	if tagsResponse.Tags[0] != tag {
		t.Fatalf("tag not as expected: %q != %q", tagsResponse.Tags[0], tag)
	}
}

func putManifest(t *testing.T, msg, url string, v interface{}) *http.Response {
	var body []byte
	if sm, ok := v.(*manifest.SignedManifest); ok {
		body = sm.Raw
	} else {
		var err error
		body, err = json.MarshalIndent(v, "", "   ")
		if err != nil {
			t.Fatalf("unexpected error marshaling %v: %v", v, err)
		}
	}

	req, err := http.NewRequest("PUT", url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("error creating request for %s: %v", msg, err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("error doing put request while %s: %v", msg, err)
	}

	return resp
}

func startPushLayer(t *testing.T, ub *v2.URLBuilder, name string) string {
	layerUploadURL, err := ub.BuildBlobUploadURL(name)
	if err != nil {
		t.Fatalf("unexpected error building layer upload url: %v", err)
	}

	resp, err := http.Post(layerUploadURL, "", nil)
	if err != nil {
		t.Fatalf("unexpected error starting layer push: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, fmt.Sprintf("pushing starting layer push %v", name), resp, http.StatusAccepted)
	checkHeaders(t, resp, http.Header{
		"Location":       []string{"*"},
		"Content-Length": []string{"0"},
	})

	return resp.Header.Get("Location")
}

// pushLayer pushes the layer content returning the url on success.
func pushLayer(t *testing.T, ub *v2.URLBuilder, name string, dgst digest.Digest, uploadURLBase string, rs io.ReadSeeker) string {
	rsLength, _ := rs.Seek(0, os.SEEK_END)
	rs.Seek(0, os.SEEK_SET)

	u, err := url.Parse(uploadURLBase)
	if err != nil {
		t.Fatalf("unexpected error parsing pushLayer url: %v", err)
	}

	u.RawQuery = url.Values{
		"_state": u.Query()["_state"],

		"digest": []string{dgst.String()},

		// TODO(stevvooe): Layer upload can be completed with and without size
		// argument. We'll need to add a test that checks the latter path.
		"size": []string{fmt.Sprint(rsLength)},
	}.Encode()

	uploadURL := u.String()

	// Just do a monolithic upload
	req, err := http.NewRequest("PUT", uploadURL, rs)
	if err != nil {
		t.Fatalf("unexpected error creating new request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error doing put: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "putting monolithic chunk", resp, http.StatusCreated)

	expectedLayerURL, err := ub.BuildBlobURL(name, dgst)
	if err != nil {
		t.Fatalf("error building expected layer url: %v", err)
	}

	checkHeaders(t, resp, http.Header{
		"Location":       []string{expectedLayerURL},
		"Content-Length": []string{"0"},
	})

	return resp.Header.Get("Location")
}

func checkResponse(t *testing.T, msg string, resp *http.Response, expectedStatus int) {
	if resp.StatusCode != expectedStatus {
		t.Logf("unexpected status %s: %v != %v", msg, resp.StatusCode, expectedStatus)
		maybeDumpResponse(t, resp)

		t.FailNow()
	}
}

func maybeDumpResponse(t *testing.T, resp *http.Response) {
	if d, err := httputil.DumpResponse(resp, true); err != nil {
		t.Logf("error dumping response: %v", err)
	} else {
		t.Logf("response:\n%s", string(d))
	}
}

// matchHeaders checks that the response has at least the headers. If not, the
// test will fail. If a passed in header value is "*", any non-zero value will
// suffice as a match.
func checkHeaders(t *testing.T, resp *http.Response, headers http.Header) {
	for k, vs := range headers {
		if resp.Header.Get(k) == "" {
			t.Fatalf("response missing header %q", k)
		}

		for _, v := range vs {
			if v == "*" {
				// Just ensure there is some value.
				if len(resp.Header[k]) > 0 {
					continue
				}
			}

			for _, hv := range resp.Header[k] {
				if hv != v {
					t.Fatalf("header value not matched in response: %q != %q", hv, v)
				}
			}
		}
	}
}
