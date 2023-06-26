package azure

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"testing"

	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/testsuites"
	"gopkg.in/check.v1"
)

const (
	envAccountName   = "AZURE_STORAGE_ACCOUNT_NAME"
	envAccountKey    = "AZURE_STORAGE_ACCOUNT_KEY"
	envContainer     = "AZURE_STORAGE_CONTAINER"
	envRealm         = "AZURE_STORAGE_REALM"
	envRootDirectory = "AZURE_ROOT_DIRECTORY"
)

var azureDriverConstructor func() (storagedriver.StorageDriver, error)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { check.TestingT(t) }

func init() {
	var (
		accountName   string
		accountKey    string
		container     string
		realm         string
		rootDirectory string
	)

	config := []struct {
		env       string
		value     *string
		missingOk bool
	}{
		{envAccountName, &accountName, true},
		{envAccountKey, &accountKey, true},
		{envContainer, &container, true},
		{envRealm, &realm, true},
		{envRootDirectory, &rootDirectory, true},
	}

	missing := []string{}
	for _, v := range config {
		*v.value = os.Getenv(v.env)
		if *v.value == "" && !v.missingOk {
			missing = append(missing, v.env)
		}
	}

	azureDriverConstructor = func() (storagedriver.StorageDriver, error) {
		parameters := map[string]interface{}{
			"container":     container,
			"accountname":   accountName,
			"accountkey":    accountKey,
			"realm":         realm,
			"rootdirectory": rootDirectory,
		}
		params, err := NewParameters(parameters)
		if err != nil {
			return nil, err
		}
		return New(params)
	}

	// Skip Azure storage driver tests if environment variable parameters are not provided
	skipCheck := func() string {
		if len(missing) > 0 {
			return fmt.Sprintf("Must set %s environment variables to run Azure tests", strings.Join(missing, ", "))
		}
		return ""
	}

	testsuites.RegisterSuite(azureDriverConstructor, skipCheck)
}

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randStringRunes(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func TestCommitAfterMove(t *testing.T) {
	driver, err := azureDriverConstructor()
	if err != nil {
		t.Fatalf("unexpected error creating azure driver: %v", err)
	}

	contents := randStringRunes(4 * 1024 * 1024)
	sourcePath := "/source/file"
	destPath := "/dest/file"
	ctx := context.Background()

	defer driver.Delete(ctx, sourcePath)
	defer driver.Delete(ctx, destPath)

	writer, err := driver.Writer(ctx, sourcePath, false)
	if err != nil {
		t.Fatalf("unexpected error from driver.Writer: %v", err)
	}

	_, err = writer.Write([]byte(contents))
	if err != nil {
		t.Fatalf("writer.Write: unexpected error: %v", err)
	}

	err = writer.Commit()
	if err != nil {
		t.Fatalf("writer.Commit: unexpected error: %v", err)
	}

	err = writer.Close()
	if err != nil {
		t.Fatalf("writer.Close: unexpected error: %v", err)
	}

	_, err = driver.GetContent(ctx, sourcePath)
	if err != nil {
		t.Fatalf("driver.GetContent(sourcePath): unexpected error: %v", err)
	}

	err = driver.Move(ctx, sourcePath, destPath)
	if err != nil {
		t.Fatalf("driver.Move: unexpected error: %v", err)
	}

	_, err = driver.GetContent(ctx, destPath)
	if err != nil {
		t.Fatalf("GetContent(destPath): unexpected error: %v", err)
	}
}

func TestParamParsing(t *testing.T) {
	expectErrors := []map[string]interface{}{
		{},
		{"accountname": "acc1"},
	}
	for _, parameters := range expectErrors {
		if _, err := NewParameters(parameters); err == nil {
			t.Fatalf("Expected an error for parameter set: %v", parameters)
		}
	}
	input := []map[string]interface{}{
		{"accountname": "acc1", "accountkey": "k1", "container": "c1"},
		{"accountname": "acc1", "container": "c1", "credentials": map[string]interface{}{"type": "default"}},
		{"accountname": "acc1", "container": "c1", "credentials": map[string]interface{}{"type": "client_secret", "clientid": "c1", "tenantid": "t1", "secret": "s1"}},
	}
	expecteds := []Parameters{
		{
			Container: "c1", AccountName: "acc1", AccountKey: "k1",
			Realm: "core.windows.net", ServiceURL: "https://acc1.blob.core.windows.net",
		},
		{
			Container: "c1", AccountName: "acc1", Credentials: Credentials{Type: "default"},
			Realm: "core.windows.net", ServiceURL: "https://acc1.blob.core.windows.net",
		},
		{
			Container: "c1", AccountName: "acc1",
			Credentials: Credentials{Type: "client_secret", ClientID: "c1", TenantID: "t1", Secret: "s1"},
			Realm:       "core.windows.net", ServiceURL: "https://acc1.blob.core.windows.net",
		},
	}
	for i, expected := range expecteds {
		actual, err := NewParameters(input[i])
		if err != nil {
			t.Fatalf("Failed to parse: %v", input[i])
		}
		if *actual != expected {
			t.Fatalf("Expected: %v != %v", *actual, expected)
		}
	}
}
