package azure

import (
	"fmt"
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
		{envAccountName, &accountName, false},
		{envAccountKey, &accountKey, false},
		{envContainer, &container, false},
		{envRealm, &realm, false},
		{envRootDirectory, &rootDirectory, true},
	}

	missing := []string{}
	for _, v := range config {
		*v.value = os.Getenv(v.env)
		if *v.value == "" && !v.missingOk {
			missing = append(missing, v.env)
		}
	}

	azureDriverConstructor := func() (storagedriver.StorageDriver, error) {
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
