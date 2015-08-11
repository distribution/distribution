package handlers

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
	"path"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/docker/distribution/configuration"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest"
	"github.com/docker/distribution/registry/api/errcode"
	"github.com/docker/distribution/registry/api/v2"
	_ "github.com/docker/distribution/registry/storage/driver/inmemory"
	"github.com/docker/distribution/testutil"
	"github.com/docker/libtrust"
	"github.com/gorilla/handlers"
	"golang.org/x/net/context"
)

// TestCheckAPI hits the base endpoint (/v2/) ensures we return the specified
// 200 OK response.
func TestCheckAPI(t *testing.T) {
	env := newTestEnv(t, false)

	baseURL, err := env.builder.BuildBaseURL()
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

// TestCatalogAPI tests the /v2/_catalog endpoint
func TestCatalogAPI(t *testing.T) {
	chunkLen := 2
	env := newTestEnv(t, false)

	values := url.Values{
		"last": []string{""},
		"n":    []string{strconv.Itoa(chunkLen)}}

	catalogURL, err := env.builder.BuildCatalogURL(values)
	if err != nil {
		t.Fatalf("unexpected error building catalog url: %v", err)
	}

	// -----------------------------------
	// try to get an empty catalog
	resp, err := http.Get(catalogURL)
	if err != nil {
		t.Fatalf("unexpected error issuing request: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "issuing catalog api check", resp, http.StatusOK)

	var ctlg struct {
		Repositories []string `json:"repositories"`
	}

	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&ctlg); err != nil {
		t.Fatalf("error decoding fetched manifest: %v", err)
	}

	// we haven't pushed anything to the registry yet
	if len(ctlg.Repositories) != 0 {
		t.Fatalf("repositories has unexpected values")
	}

	if resp.Header.Get("Link") != "" {
		t.Fatalf("repositories has more data when none expected")
	}

	// -----------------------------------
	// push something to the registry and try again
	images := []string{"foo/aaaa", "foo/bbbb", "foo/cccc"}

	for _, image := range images {
		createRepository(env, t, image, "sometag")
	}

	resp, err = http.Get(catalogURL)
	if err != nil {
		t.Fatalf("unexpected error issuing request: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "issuing catalog api check", resp, http.StatusOK)

	dec = json.NewDecoder(resp.Body)
	if err = dec.Decode(&ctlg); err != nil {
		t.Fatalf("error decoding fetched manifest: %v", err)
	}

	if len(ctlg.Repositories) != chunkLen {
		t.Fatalf("repositories has unexpected values")
	}

	for _, image := range images[:chunkLen] {
		if !contains(ctlg.Repositories, image) {
			t.Fatalf("didn't find our repository '%s' in the catalog", image)
		}
	}

	link := resp.Header.Get("Link")
	if link == "" {
		t.Fatalf("repositories has less data than expected")
	}

	newValues := checkLink(t, link, chunkLen, ctlg.Repositories[len(ctlg.Repositories)-1])

	// -----------------------------------
	// get the last chunk of data

	catalogURL, err = env.builder.BuildCatalogURL(newValues)
	if err != nil {
		t.Fatalf("unexpected error building catalog url: %v", err)
	}

	resp, err = http.Get(catalogURL)
	if err != nil {
		t.Fatalf("unexpected error issuing request: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "issuing catalog api check", resp, http.StatusOK)

	dec = json.NewDecoder(resp.Body)
	if err = dec.Decode(&ctlg); err != nil {
		t.Fatalf("error decoding fetched manifest: %v", err)
	}

	if len(ctlg.Repositories) != 1 {
		t.Fatalf("repositories has unexpected values")
	}

	lastImage := images[len(images)-1]
	if !contains(ctlg.Repositories, lastImage) {
		t.Fatalf("didn't find our repository '%s' in the catalog", lastImage)
	}

	link = resp.Header.Get("Link")
	if link != "" {
		t.Fatalf("catalog has unexpected data")
	}
}

func checkLink(t *testing.T, urlStr string, numEntries int, last string) url.Values {
	re := regexp.MustCompile("<(/v2/_catalog.*)>; rel=\"next\"")
	matches := re.FindStringSubmatch(urlStr)

	if len(matches) != 2 {
		t.Fatalf("Catalog link address response was incorrect")
	}
	linkURL, _ := url.Parse(matches[1])
	urlValues := linkURL.Query()

	if urlValues.Get("n") != strconv.Itoa(numEntries) {
		t.Fatalf("Catalog link entry size is incorrect")
	}

	if urlValues.Get("last") != last {
		t.Fatal("Catalog link last entry is incorrect")
	}

	return urlValues
}

func contains(elems []string, e string) bool {
	for _, elem := range elems {
		if elem == e {
			return true
		}
	}
	return false
}

func TestURLPrefix(t *testing.T) {
	config := configuration.Configuration{
		Storage: configuration.Storage{
			"inmemory": configuration.Parameters{},
		},
	}
	config.HTTP.Prefix = "/test/"

	env := newTestEnvWithConfig(t, &config)

	baseURL, err := env.builder.BuildBaseURL()
	if err != nil {
		t.Fatalf("unexpected error building base url: %v", err)
	}

	parsed, _ := url.Parse(baseURL)
	if !strings.HasPrefix(parsed.Path, config.HTTP.Prefix) {
		t.Fatalf("Prefix %v not included in test url %v", config.HTTP.Prefix, baseURL)
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
}

type blobArgs struct {
	imageName   string
	layerFile   io.ReadSeeker
	layerDigest digest.Digest
	tarSumStr   string
}

func makeBlobArgs(t *testing.T) blobArgs {
	layerFile, tarSumStr, err := testutil.CreateRandomTarFile()
	if err != nil {
		t.Fatalf("error creating random layer file: %v", err)
	}

	layerDigest := digest.Digest(tarSumStr)

	args := blobArgs{
		imageName:   "foo/bar",
		layerFile:   layerFile,
		layerDigest: layerDigest,
		tarSumStr:   tarSumStr,
	}
	return args
}

// TestBlobAPI conducts a full test of the of the blob api.
func TestBlobAPI(t *testing.T) {
	deleteEnabled := false
	env := newTestEnv(t, deleteEnabled)
	args := makeBlobArgs(t)
	testBlobAPI(t, env, args)

	deleteEnabled = true
	env = newTestEnv(t, deleteEnabled)
	args = makeBlobArgs(t)
	testBlobAPI(t, env, args)

}

func TestBlobDelete(t *testing.T) {
	deleteEnabled := true
	env := newTestEnv(t, deleteEnabled)

	args := makeBlobArgs(t)
	env = testBlobAPI(t, env, args)
	testBlobDelete(t, env, args)
}

func TestBlobDeleteDisabled(t *testing.T) {
	deleteEnabled := false
	env := newTestEnv(t, deleteEnabled)
	args := makeBlobArgs(t)

	imageName := args.imageName
	layerDigest := args.layerDigest
	layerURL, err := env.builder.BuildBlobURL(imageName, layerDigest)
	if err != nil {
		t.Fatalf("error building url: %v", err)
	}

	resp, err := httpDelete(layerURL)
	if err != nil {
		t.Fatalf("unexpected error deleting when disabled: %v", err)
	}

	checkResponse(t, "status of disabled delete", resp, http.StatusMethodNotAllowed)
}

func testBlobAPI(t *testing.T, env *testEnv, args blobArgs) *testEnv {
	// TODO(stevvooe): This test code is complete junk but it should cover the
	// complete flow. This must be broken down and checked against the
	// specification *before* we submit the final to docker core.
	imageName := args.imageName
	layerFile := args.layerFile
	layerDigest := args.layerDigest

	// -----------------------------------
	// Test fetch for non-existent content
	layerURL, err := env.builder.BuildBlobURL(imageName, layerDigest)
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
	// Start an upload, check the status then cancel
	uploadURLBase, uploadUUID := startPushLayer(t, env.builder, imageName)

	// A status check should work
	resp, err = http.Get(uploadURLBase)
	if err != nil {
		t.Fatalf("unexpected error getting upload status: %v", err)
	}
	checkResponse(t, "status of deleted upload", resp, http.StatusNoContent)
	checkHeaders(t, resp, http.Header{
		"Location":           []string{"*"},
		"Range":              []string{"0-0"},
		"Docker-Upload-UUID": []string{uploadUUID},
	})

	req, err := http.NewRequest("DELETE", uploadURLBase, nil)
	if err != nil {
		t.Fatalf("unexpected error creating delete request: %v", err)
	}

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error sending delete request: %v", err)
	}

	checkResponse(t, "deleting upload", resp, http.StatusNoContent)

	// A status check should result in 404
	resp, err = http.Get(uploadURLBase)
	if err != nil {
		t.Fatalf("unexpected error getting upload status: %v", err)
	}
	checkResponse(t, "status of deleted upload", resp, http.StatusNotFound)

	// -----------------------------------------
	// Do layer push with an empty body and different digest
	uploadURLBase, uploadUUID = startPushLayer(t, env.builder, imageName)
	resp, err = doPushLayer(t, env.builder, imageName, layerDigest, uploadURLBase, bytes.NewReader([]byte{}))
	if err != nil {
		t.Fatalf("unexpected error doing bad layer push: %v", err)
	}

	checkResponse(t, "bad layer push", resp, http.StatusBadRequest)
	checkBodyHasErrorCodes(t, "bad layer push", resp, v2.ErrorCodeDigestInvalid)

	// -----------------------------------------
	// Do layer push with an empty body and correct digest
	zeroDigest, err := digest.FromTarArchive(bytes.NewReader([]byte{}))
	if err != nil {
		t.Fatalf("unexpected error digesting empty buffer: %v", err)
	}

	uploadURLBase, uploadUUID = startPushLayer(t, env.builder, imageName)
	pushLayer(t, env.builder, imageName, zeroDigest, uploadURLBase, bytes.NewReader([]byte{}))

	// -----------------------------------------
	// Do layer push with an empty body and correct digest

	// This is a valid but empty tarfile!
	emptyTar := bytes.Repeat([]byte("\x00"), 1024)
	emptyDigest, err := digest.FromTarArchive(bytes.NewReader(emptyTar))
	if err != nil {
		t.Fatalf("unexpected error digesting empty tar: %v", err)
	}

	uploadURLBase, uploadUUID = startPushLayer(t, env.builder, imageName)
	pushLayer(t, env.builder, imageName, emptyDigest, uploadURLBase, bytes.NewReader(emptyTar))

	// ------------------------------------------
	// Now, actually do successful upload.
	layerLength, _ := layerFile.Seek(0, os.SEEK_END)
	layerFile.Seek(0, os.SEEK_SET)

	uploadURLBase, uploadUUID = startPushLayer(t, env.builder, imageName)
	pushLayer(t, env.builder, imageName, layerDigest, uploadURLBase, layerFile)

	// ------------------------------------------
	// Now, push just a chunk
	layerFile.Seek(0, 0)

	canonicalDigester := digest.Canonical.New()
	if _, err := io.Copy(canonicalDigester.Hash(), layerFile); err != nil {
		t.Fatalf("error copying to digest: %v", err)
	}
	canonicalDigest := canonicalDigester.Digest()

	layerFile.Seek(0, 0)
	uploadURLBase, uploadUUID = startPushLayer(t, env.builder, imageName)
	uploadURLBase, dgst := pushChunk(t, env.builder, imageName, uploadURLBase, layerFile, layerLength)
	finishUpload(t, env.builder, imageName, uploadURLBase, dgst)

	// ------------------------
	// Use a head request to see if the layer exists.
	resp, err = http.Head(layerURL)
	if err != nil {
		t.Fatalf("unexpected error checking head on existing layer: %v", err)
	}

	checkResponse(t, "checking head on existing layer", resp, http.StatusOK)
	checkHeaders(t, resp, http.Header{
		"Content-Length":        []string{fmt.Sprint(layerLength)},
		"Docker-Content-Digest": []string{canonicalDigest.String()},
	})

	// ----------------
	// Fetch the layer!
	resp, err = http.Get(layerURL)
	if err != nil {
		t.Fatalf("unexpected error fetching layer: %v", err)
	}

	checkResponse(t, "fetching layer", resp, http.StatusOK)
	checkHeaders(t, resp, http.Header{
		"Content-Length":        []string{fmt.Sprint(layerLength)},
		"Docker-Content-Digest": []string{canonicalDigest.String()},
	})

	// Verify the body
	verifier, err := digest.NewDigestVerifier(layerDigest)
	if err != nil {
		t.Fatalf("unexpected error getting digest verifier: %s", err)
	}
	io.Copy(verifier, resp.Body)

	if !verifier.Verified() {
		t.Fatalf("response body did not pass verification")
	}

	// ----------------
	// Fetch the layer with an invalid digest
	badURL := strings.Replace(layerURL, "tarsum", "trsum", 1)
	resp, err = http.Get(badURL)
	if err != nil {
		t.Fatalf("unexpected error fetching layer: %v", err)
	}

	checkResponse(t, "fetching layer bad digest", resp, http.StatusBadRequest)

	// Cache headers
	resp, err = http.Get(layerURL)
	if err != nil {
		t.Fatalf("unexpected error fetching layer: %v", err)
	}

	checkResponse(t, "fetching layer", resp, http.StatusOK)
	checkHeaders(t, resp, http.Header{
		"Content-Length":        []string{fmt.Sprint(layerLength)},
		"Docker-Content-Digest": []string{canonicalDigest.String()},
		"ETag":                  []string{fmt.Sprintf(`"%s"`, canonicalDigest)},
		"Cache-Control":         []string{"max-age=31536000"},
	})

	// Matching etag, gives 304
	etag := resp.Header.Get("Etag")
	req, err = http.NewRequest("GET", layerURL, nil)
	if err != nil {
		t.Fatalf("Error constructing request: %s", err)
	}
	req.Header.Set("If-None-Match", etag)

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Error constructing request: %s", err)
	}

	checkResponse(t, "fetching layer with etag", resp, http.StatusNotModified)

	// Non-matching etag, gives 200
	req, err = http.NewRequest("GET", layerURL, nil)
	if err != nil {
		t.Fatalf("Error constructing request: %s", err)
	}
	req.Header.Set("If-None-Match", "")
	resp, err = http.DefaultClient.Do(req)
	checkResponse(t, "fetching layer with invalid etag", resp, http.StatusOK)

	// Missing tests:
	// 	- Upload the same tarsum file under and different repository and
	//       ensure the content remains uncorrupted.
	return env
}

func testBlobDelete(t *testing.T, env *testEnv, args blobArgs) {
	// Upload a layer
	imageName := args.imageName
	layerFile := args.layerFile
	layerDigest := args.layerDigest

	layerURL, err := env.builder.BuildBlobURL(imageName, layerDigest)
	if err != nil {
		t.Fatalf(err.Error())
	}
	// ---------------
	// Delete a layer
	resp, err := httpDelete(layerURL)
	if err != nil {
		t.Fatalf("unexpected error deleting layer: %v", err)
	}

	checkResponse(t, "deleting layer", resp, http.StatusAccepted)
	checkHeaders(t, resp, http.Header{
		"Content-Length": []string{"0"},
	})

	// ---------------
	// Try and get it back
	// Use a head request to see if the layer exists.
	resp, err = http.Head(layerURL)
	if err != nil {
		t.Fatalf("unexpected error checking head on existing layer: %v", err)
	}

	checkResponse(t, "checking existence of deleted layer", resp, http.StatusNotFound)

	// Delete already deleted layer
	resp, err = httpDelete(layerURL)
	if err != nil {
		t.Fatalf("unexpected error deleting layer: %v", err)
	}

	checkResponse(t, "deleting layer", resp, http.StatusNotFound)

	// ----------------
	// Attempt to delete a layer with an invalid digest
	badURL := strings.Replace(layerURL, "tarsum", "trsum", 1)
	resp, err = httpDelete(badURL)
	if err != nil {
		t.Fatalf("unexpected error fetching layer: %v", err)
	}

	checkResponse(t, "deleting layer bad digest", resp, http.StatusBadRequest)

	// ----------------
	// Reupload previously deleted blob
	layerFile.Seek(0, os.SEEK_SET)

	uploadURLBase, _ := startPushLayer(t, env.builder, imageName)
	pushLayer(t, env.builder, imageName, layerDigest, uploadURLBase, layerFile)

	layerFile.Seek(0, os.SEEK_SET)
	canonicalDigester := digest.Canonical.New()
	if _, err := io.Copy(canonicalDigester.Hash(), layerFile); err != nil {
		t.Fatalf("error copying to digest: %v", err)
	}
	canonicalDigest := canonicalDigester.Digest()

	// ------------------------
	// Use a head request to see if it exists
	resp, err = http.Head(layerURL)
	if err != nil {
		t.Fatalf("unexpected error checking head on existing layer: %v", err)
	}

	layerLength, _ := layerFile.Seek(0, os.SEEK_END)
	checkResponse(t, "checking head on reuploaded layer", resp, http.StatusOK)
	checkHeaders(t, resp, http.Header{
		"Content-Length":        []string{fmt.Sprint(layerLength)},
		"Docker-Content-Digest": []string{canonicalDigest.String()},
	})
}

func TestDeleteDisabled(t *testing.T) {
	env := newTestEnv(t, false)

	imageName := "foo/bar"
	// "build" our layer file
	layerFile, tarSumStr, err := testutil.CreateRandomTarFile()
	if err != nil {
		t.Fatalf("error creating random layer file: %v", err)
	}

	layerDigest := digest.Digest(tarSumStr)
	layerURL, err := env.builder.BuildBlobURL(imageName, layerDigest)
	if err != nil {
		t.Fatalf("Error building blob URL")
	}
	uploadURLBase, _ := startPushLayer(t, env.builder, imageName)
	pushLayer(t, env.builder, imageName, layerDigest, uploadURLBase, layerFile)

	resp, err := httpDelete(layerURL)
	if err != nil {
		t.Fatalf("unexpected error deleting layer: %v", err)
	}

	checkResponse(t, "deleting layer with delete disabled", resp, http.StatusMethodNotAllowed)
}

func httpDelete(url string) (*http.Response, error) {
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	//	defer resp.Body.Close()
	return resp, err
}

type manifestArgs struct {
	imageName      string
	signedManifest *manifest.SignedManifest
	dgst           digest.Digest
}

func makeManifestArgs(t *testing.T) manifestArgs {
	args := manifestArgs{
		imageName: "foo/bar",
	}

	return args
}

func TestManifestAPI(t *testing.T) {
	deleteEnabled := false
	env := newTestEnv(t, deleteEnabled)
	args := makeManifestArgs(t)
	testManifestAPI(t, env, args)

	deleteEnabled = true
	env = newTestEnv(t, deleteEnabled)
	args = makeManifestArgs(t)
	testManifestAPI(t, env, args)
}

func TestManifestDelete(t *testing.T) {
	deleteEnabled := true
	env := newTestEnv(t, deleteEnabled)
	args := makeManifestArgs(t)
	env, args = testManifestAPI(t, env, args)
	testManifestDelete(t, env, args)
}

func TestManifestDeleteDisabled(t *testing.T) {
	deleteEnabled := false
	env := newTestEnv(t, deleteEnabled)
	args := makeManifestArgs(t)
	testManifestDeleteDisabled(t, env, args)
}

func testManifestDeleteDisabled(t *testing.T, env *testEnv, args manifestArgs) *testEnv {
	imageName := args.imageName
	manifestURL, err := env.builder.BuildManifestURL(imageName, digest.DigestSha256EmptyTar)
	if err != nil {
		t.Fatalf("unexpected error getting manifest url: %v", err)
	}

	resp, err := httpDelete(manifestURL)
	if err != nil {
		t.Fatalf("unexpected error deleting manifest %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "status of disabled delete of manifest", resp, http.StatusMethodNotAllowed)
	return nil
}

func testManifestAPI(t *testing.T, env *testEnv, args manifestArgs) (*testEnv, manifestArgs) {
	imageName := args.imageName
	tag := "thetag"

	manifestURL, err := env.builder.BuildManifestURL(imageName, tag)
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
	checkBodyHasErrorCodes(t, "getting non-existent manifest", resp, v2.ErrorCodeManifestUnknown)

	tagsURL, err := env.builder.BuildTagsURL(imageName)
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
	checkBodyHasErrorCodes(t, "getting unknown manifest tags", resp, v2.ErrorCodeNameUnknown)

	// --------------------------------
	// Attempt to push unsigned manifest with missing layers
	unsignedManifest := &manifest.Manifest{
		Versioned: manifest.Versioned{
			SchemaVersion: 1,
		},
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
	_, p, counts := checkBodyHasErrorCodes(t, "getting unknown manifest tags", resp,
		v2.ErrorCodeManifestUnverified, v2.ErrorCodeBlobUnknown, v2.ErrorCodeDigestInvalid)

	expectedCounts := map[errcode.ErrorCode]int{
		v2.ErrorCodeManifestUnverified: 1,
		v2.ErrorCodeBlobUnknown:        2,
		v2.ErrorCodeDigestInvalid:      2,
	}

	if !reflect.DeepEqual(counts, expectedCounts) {
		t.Fatalf("unexpected number of error codes encountered: %v\n!=\n%v\n---\n%s", counts, expectedCounts, string(p))
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

		uploadURLBase, _ := startPushLayer(t, env.builder, imageName)
		pushLayer(t, env.builder, imageName, dgst, uploadURLBase, rs)
	}

	// -------------------
	// Push the signed manifest with all layers pushed.
	signedManifest, err := manifest.Sign(unsignedManifest, env.pk)
	if err != nil {
		t.Fatalf("unexpected error signing manifest: %v", err)
	}

	payload, err := signedManifest.Payload()
	checkErr(t, err, "getting manifest payload")

	dgst, err := digest.FromBytes(payload)
	checkErr(t, err, "digesting manifest")

	args.signedManifest = signedManifest
	args.dgst = dgst

	manifestDigestURL, err := env.builder.BuildManifestURL(imageName, dgst.String())
	checkErr(t, err, "building manifest url")

	resp = putManifest(t, "putting signed manifest", manifestURL, signedManifest)
	checkResponse(t, "putting signed manifest", resp, http.StatusCreated)
	checkHeaders(t, resp, http.Header{
		"Location":              []string{manifestDigestURL},
		"Docker-Content-Digest": []string{dgst.String()},
	})

	// --------------------
	// Push by digest -- should get same result
	resp = putManifest(t, "putting signed manifest", manifestDigestURL, signedManifest)
	checkResponse(t, "putting signed manifest", resp, http.StatusCreated)
	checkHeaders(t, resp, http.Header{
		"Location":              []string{manifestDigestURL},
		"Docker-Content-Digest": []string{dgst.String()},
	})

	// ------------------
	// Fetch by tag name
	resp, err = http.Get(manifestURL)
	if err != nil {
		t.Fatalf("unexpected error fetching manifest: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "fetching uploaded manifest", resp, http.StatusOK)
	checkHeaders(t, resp, http.Header{
		"Docker-Content-Digest": []string{dgst.String()},
		"ETag":                  []string{fmt.Sprintf(`"%s"`, dgst)},
	})

	var fetchedManifest manifest.SignedManifest
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&fetchedManifest); err != nil {
		t.Fatalf("error decoding fetched manifest: %v", err)
	}

	if !bytes.Equal(fetchedManifest.Raw, signedManifest.Raw) {
		t.Fatalf("manifests do not match")
	}

	// ---------------
	// Fetch by digest
	resp, err = http.Get(manifestDigestURL)
	checkErr(t, err, "fetching manifest by digest")
	defer resp.Body.Close()

	checkResponse(t, "fetching uploaded manifest", resp, http.StatusOK)
	checkHeaders(t, resp, http.Header{
		"Docker-Content-Digest": []string{dgst.String()},
		"ETag":                  []string{fmt.Sprintf(`"%s"`, dgst)},
	})

	var fetchedManifestByDigest manifest.SignedManifest
	dec = json.NewDecoder(resp.Body)
	if err := dec.Decode(&fetchedManifestByDigest); err != nil {
		t.Fatalf("error decoding fetched manifest: %v", err)
	}

	if !bytes.Equal(fetchedManifestByDigest.Raw, signedManifest.Raw) {
		t.Fatalf("manifests do not match")
	}

	// Get by name with etag, gives 304
	etag := resp.Header.Get("Etag")
	req, err := http.NewRequest("GET", manifestURL, nil)
	if err != nil {
		t.Fatalf("Error constructing request: %s", err)
	}
	req.Header.Set("If-None-Match", etag)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Error constructing request: %s", err)
	}

	checkResponse(t, "fetching layer with etag", resp, http.StatusNotModified)

	// Get by digest with etag, gives 304
	req, err = http.NewRequest("GET", manifestDigestURL, nil)
	if err != nil {
		t.Fatalf("Error constructing request: %s", err)
	}
	req.Header.Set("If-None-Match", etag)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Error constructing request: %s", err)
	}

	checkResponse(t, "fetching layer with etag", resp, http.StatusNotModified)

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

	return env, args
}

func testManifestDelete(t *testing.T, env *testEnv, args manifestArgs) {
	imageName := args.imageName
	dgst := args.dgst
	signedManifest := args.signedManifest
	manifestDigestURL, err := env.builder.BuildManifestURL(imageName, dgst.String())
	// ---------------
	// Delete by digest
	resp, err := httpDelete(manifestDigestURL)
	checkErr(t, err, "deleting manifest by digest")

	checkResponse(t, "deleting manifest", resp, http.StatusAccepted)
	checkHeaders(t, resp, http.Header{
		"Content-Length": []string{"0"},
	})

	// ---------------
	// Attempt to fetch deleted manifest
	resp, err = http.Get(manifestDigestURL)
	checkErr(t, err, "fetching deleted manifest by digest")
	defer resp.Body.Close()

	checkResponse(t, "fetching deleted manifest", resp, http.StatusNotFound)

	// ---------------
	// Delete already deleted manifest by digest
	resp, err = httpDelete(manifestDigestURL)
	checkErr(t, err, "re-deleting manifest by digest")

	checkResponse(t, "re-deleting manifest", resp, http.StatusNotFound)

	// --------------------
	// Re-upload manifest by digest
	resp = putManifest(t, "putting signed manifest", manifestDigestURL, signedManifest)
	checkResponse(t, "putting signed manifest", resp, http.StatusCreated)
	checkHeaders(t, resp, http.Header{
		"Location":              []string{manifestDigestURL},
		"Docker-Content-Digest": []string{dgst.String()},
	})

	// ---------------
	// Attempt to fetch re-uploaded deleted digest
	resp, err = http.Get(manifestDigestURL)
	checkErr(t, err, "fetching re-uploaded manifest by digest")
	defer resp.Body.Close()

	checkResponse(t, "fetching re-uploaded manifest", resp, http.StatusOK)
	checkHeaders(t, resp, http.Header{
		"Docker-Content-Digest": []string{dgst.String()},
	})

	// ---------------
	// Attempt to delete an unknown manifest
	unknownDigest := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	unknownManifestDigestURL, err := env.builder.BuildManifestURL(imageName, unknownDigest)
	checkErr(t, err, "building unknown manifest url")

	resp, err = httpDelete(unknownManifestDigestURL)
	checkErr(t, err, "delting unknown manifest by digest")
	checkResponse(t, "fetching deleted manifest", resp, http.StatusNotFound)

}

type testEnv struct {
	pk      libtrust.PrivateKey
	ctx     context.Context
	config  configuration.Configuration
	app     *App
	server  *httptest.Server
	builder *v2.URLBuilder
}

func newTestEnvMirror(t *testing.T, deleteEnabled bool) *testEnv {
	config := configuration.Configuration{
		Storage: configuration.Storage{
			"inmemory": configuration.Parameters{},
			"delete":   configuration.Parameters{"enabled": deleteEnabled},
		},
		Proxy: configuration.Proxy{
			RemoteURL: "http://example.com",
		},
	}

	return newTestEnvWithConfig(t, &config)

}

func newTestEnv(t *testing.T, deleteEnabled bool) *testEnv {
	config := configuration.Configuration{
		Storage: configuration.Storage{
			"inmemory": configuration.Parameters{},
			"delete":   configuration.Parameters{"enabled": deleteEnabled},
		},
	}

	return newTestEnvWithConfig(t, &config)
}

func newTestEnvWithConfig(t *testing.T, config *configuration.Configuration) *testEnv {
	ctx := context.Background()

	app := NewApp(ctx, *config)
	server := httptest.NewServer(handlers.CombinedLoggingHandler(os.Stderr, app))
	builder, err := v2.NewURLBuilderFromString(server.URL + config.HTTP.Prefix)

	if err != nil {
		t.Fatalf("error creating url builder: %v", err)
	}

	pk, err := libtrust.GenerateECP256PrivateKey()
	if err != nil {
		t.Fatalf("unexpected error generating private key: %v", err)
	}

	return &testEnv{
		pk:      pk,
		ctx:     ctx,
		config:  *config,
		app:     app,
		server:  server,
		builder: builder,
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

func startPushLayer(t *testing.T, ub *v2.URLBuilder, name string) (location string, uuid string) {
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

	u, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("error parsing location header: %v", err)
	}

	uuid = path.Base(u.Path)
	checkHeaders(t, resp, http.Header{
		"Location":           []string{"*"},
		"Content-Length":     []string{"0"},
		"Docker-Upload-UUID": []string{uuid},
	})

	return resp.Header.Get("Location"), uuid
}

// doPushLayer pushes the layer content returning the url on success returning
// the response. If you're only expecting a successful response, use pushLayer.
func doPushLayer(t *testing.T, ub *v2.URLBuilder, name string, dgst digest.Digest, uploadURLBase string, body io.Reader) (*http.Response, error) {
	u, err := url.Parse(uploadURLBase)
	if err != nil {
		t.Fatalf("unexpected error parsing pushLayer url: %v", err)
	}

	u.RawQuery = url.Values{
		"_state": u.Query()["_state"],

		"digest": []string{dgst.String()},
	}.Encode()

	uploadURL := u.String()

	// Just do a monolithic upload
	req, err := http.NewRequest("PUT", uploadURL, body)
	if err != nil {
		t.Fatalf("unexpected error creating new request: %v", err)
	}

	return http.DefaultClient.Do(req)
}

// pushLayer pushes the layer content returning the url on success.
func pushLayer(t *testing.T, ub *v2.URLBuilder, name string, dgst digest.Digest, uploadURLBase string, body io.Reader) string {
	digester := digest.Canonical.New()

	resp, err := doPushLayer(t, ub, name, dgst, uploadURLBase, io.TeeReader(body, digester.Hash()))
	if err != nil {
		t.Fatalf("unexpected error doing push layer request: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "putting monolithic chunk", resp, http.StatusCreated)

	if err != nil {
		t.Fatalf("error generating sha256 digest of body")
	}

	sha256Dgst := digester.Digest()

	expectedLayerURL, err := ub.BuildBlobURL(name, sha256Dgst)
	if err != nil {
		t.Fatalf("error building expected layer url: %v", err)
	}

	checkHeaders(t, resp, http.Header{
		"Location":              []string{expectedLayerURL},
		"Content-Length":        []string{"0"},
		"Docker-Content-Digest": []string{sha256Dgst.String()},
	})

	return resp.Header.Get("Location")
}

func finishUpload(t *testing.T, ub *v2.URLBuilder, name string, uploadURLBase string, dgst digest.Digest) string {
	resp, err := doPushLayer(t, ub, name, dgst, uploadURLBase, nil)
	if err != nil {
		t.Fatalf("unexpected error doing push layer request: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "putting monolithic chunk", resp, http.StatusCreated)

	expectedLayerURL, err := ub.BuildBlobURL(name, dgst)
	if err != nil {
		t.Fatalf("error building expected layer url: %v", err)
	}

	checkHeaders(t, resp, http.Header{
		"Location":              []string{expectedLayerURL},
		"Content-Length":        []string{"0"},
		"Docker-Content-Digest": []string{dgst.String()},
	})

	return resp.Header.Get("Location")
}

func doPushChunk(t *testing.T, uploadURLBase string, body io.Reader) (*http.Response, digest.Digest, error) {
	u, err := url.Parse(uploadURLBase)
	if err != nil {
		t.Fatalf("unexpected error parsing pushLayer url: %v", err)
	}

	u.RawQuery = url.Values{
		"_state": u.Query()["_state"],
	}.Encode()

	uploadURL := u.String()

	digester := digest.Canonical.New()

	req, err := http.NewRequest("PATCH", uploadURL, io.TeeReader(body, digester.Hash()))
	if err != nil {
		t.Fatalf("unexpected error creating new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := http.DefaultClient.Do(req)

	return resp, digester.Digest(), err
}

func pushChunk(t *testing.T, ub *v2.URLBuilder, name string, uploadURLBase string, body io.Reader, length int64) (string, digest.Digest) {
	resp, dgst, err := doPushChunk(t, uploadURLBase, body)
	if err != nil {
		t.Fatalf("unexpected error doing push layer request: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "putting chunk", resp, http.StatusAccepted)

	if err != nil {
		t.Fatalf("error generating sha256 digest of body")
	}

	checkHeaders(t, resp, http.Header{
		"Range":          []string{fmt.Sprintf("0-%d", length-1)},
		"Content-Length": []string{"0"},
	})

	return resp.Header.Get("Location"), dgst
}

func checkResponse(t *testing.T, msg string, resp *http.Response, expectedStatus int) {
	if resp.StatusCode != expectedStatus {
		t.Logf("unexpected status %s: %v != %v", msg, resp.StatusCode, expectedStatus)
		maybeDumpResponse(t, resp)

		t.FailNow()
	}
}

// checkBodyHasErrorCodes ensures the body is an error body and has the
// expected error codes, returning the error structure, the json slice and a
// count of the errors by code.
func checkBodyHasErrorCodes(t *testing.T, msg string, resp *http.Response, errorCodes ...errcode.ErrorCode) (errcode.Errors, []byte, map[errcode.ErrorCode]int) {
	p, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("unexpected error reading body %s: %v", msg, err)
	}

	var errs errcode.Errors
	if err := json.Unmarshal(p, &errs); err != nil {
		t.Fatalf("unexpected error decoding error response: %v", err)
	}

	if len(errs) == 0 {
		t.Fatalf("expected errors in response")
	}

	// TODO(stevvooe): Shoot. The error setup is not working out. The content-
	// type headers are being set after writing the status code.
	// if resp.Header.Get("Content-Type") != "application/json; charset=utf-8" {
	// 	t.Fatalf("unexpected content type: %v != 'application/json'",
	// 		resp.Header.Get("Content-Type"))
	// }

	expected := map[errcode.ErrorCode]struct{}{}
	counts := map[errcode.ErrorCode]int{}

	// Initialize map with zeros for expected
	for _, code := range errorCodes {
		expected[code] = struct{}{}
		counts[code] = 0
	}

	for _, e := range errs {
		err, ok := e.(errcode.ErrorCoder)
		if !ok {
			t.Fatalf("not an ErrorCoder: %#v", e)
		}
		if _, ok := expected[err.ErrorCode()]; !ok {
			t.Fatalf("unexpected error code %v encountered during %s: %s ", err.ErrorCode(), msg, string(p))
		}
		counts[err.ErrorCode()]++
	}

	// Ensure that counts of expected errors were all non-zero
	for code := range expected {
		if counts[code] == 0 {
			t.Fatalf("expected error code %v not encounterd during %s: %s", code, msg, string(p))
		}
	}

	return errs, p, counts
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
				if len(resp.Header[http.CanonicalHeaderKey(k)]) > 0 {
					continue
				}
			}

			for _, hv := range resp.Header[http.CanonicalHeaderKey(k)] {
				if hv != v {
					t.Fatalf("%+v %v header value not matched in response: %q != %q", resp.Header, k, hv, v)
				}
			}
		}
	}
}

func checkErr(t *testing.T, err error, msg string) {
	if err != nil {
		t.Fatalf("unexpected error %s: %v", msg, err)
	}
}

func createRepository(env *testEnv, t *testing.T, imageName string, tag string) {
	unsignedManifest := &manifest.Manifest{
		Versioned: manifest.Versioned{
			SchemaVersion: 1,
		},
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

		uploadURLBase, _ := startPushLayer(t, env.builder, imageName)
		pushLayer(t, env.builder, imageName, dgst, uploadURLBase, rs)
	}

	signedManifest, err := manifest.Sign(unsignedManifest, env.pk)
	if err != nil {
		t.Fatalf("unexpected error signing manifest: %v", err)
	}

	payload, err := signedManifest.Payload()
	checkErr(t, err, "getting manifest payload")

	dgst, err := digest.FromBytes(payload)
	checkErr(t, err, "digesting manifest")

	manifestDigestURL, err := env.builder.BuildManifestURL(imageName, dgst.String())
	checkErr(t, err, "building manifest url")

	resp := putManifest(t, "putting signed manifest", manifestDigestURL, signedManifest)
	checkResponse(t, "putting signed manifest", resp, http.StatusCreated)
	checkHeaders(t, resp, http.Header{
		"Location":              []string{manifestDigestURL},
		"Docker-Content-Digest": []string{dgst.String()},
	})
}

// Test mutation operations on a registry configured as a cache.  Ensure that they return
// appropriate errors.
func TestRegistryAsCacheMutationAPIs(t *testing.T) {
	deleteEnabled := true
	env := newTestEnvMirror(t, deleteEnabled)

	imageName := "foo/bar"
	tag := "latest"
	manifestURL, err := env.builder.BuildManifestURL(imageName, tag)
	if err != nil {
		t.Fatalf("unexpected error building base url: %v", err)
	}

	// Manifest upload
	unsignedManifest := &manifest.Manifest{
		Versioned: manifest.Versioned{
			SchemaVersion: 1,
		},
		Name:     imageName,
		Tag:      tag,
		FSLayers: []manifest.FSLayer{},
	}
	resp := putManifest(t, "putting unsigned manifest", manifestURL, unsignedManifest)
	checkResponse(t, "putting signed manifest to cache", resp, errcode.ErrorCodeUnsupported.Descriptor().HTTPStatusCode)

	// Manifest Delete
	resp, err = httpDelete(manifestURL)
	checkResponse(t, "deleting signed manifest from cache", resp, errcode.ErrorCodeUnsupported.Descriptor().HTTPStatusCode)

	// Blob upload initialization
	layerUploadURL, err := env.builder.BuildBlobUploadURL(imageName)
	if err != nil {
		t.Fatalf("unexpected error building layer upload url: %v", err)
	}

	resp, err = http.Post(layerUploadURL, "", nil)
	if err != nil {
		t.Fatalf("unexpected error starting layer push: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, fmt.Sprintf("starting layer push to cache %v", imageName), resp, errcode.ErrorCodeUnsupported.Descriptor().HTTPStatusCode)

	// Blob Delete
	blobURL, err := env.builder.BuildBlobURL(imageName, digest.DigestSha256EmptyTar)
	resp, err = httpDelete(blobURL)
	checkResponse(t, "deleting blob from cache", resp, errcode.ErrorCodeUnsupported.Descriptor().HTTPStatusCode)

}
