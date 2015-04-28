package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestCreateCertPool(t *testing.T) {
	_, err := createCertPool([]string{"foo"}, func(path string) ([]byte, error) {
		return []byte{}, fmt.Errorf("Nope")
	})

	if err == nil {
		t.Fatal("File read failure")
	}

	_, err = createCertPool([]string{"foo"}, func(path string) ([]byte, error) {
		return []byte{'f', 'o', 'o'}, nil
	})
	if err == nil {
		t.Fatal("Failed to report invalid cert")
	}

	pool, err := createCertPool([]string{"foo"}, func(path string) ([]byte, error) {
		pem := `
-----BEGIN CERTIFICATE-----
MIICyzCCAbWgAwIBAgIQFtYf4Hzz2g+UcXMe/RjhSTALBgkqhkiG9w0BAQswETEP
MA0GA1UEChMGc3RodWxiMB4XDTE1MDQyODEwMzUwMFoXDTE4MDQxMjEwMzUwMFow
ETEPMA0GA1UEChMGc3RodWxiMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKC
AQEAviiFNJ9Z7wTl6crvZjIBXcTlkphBUHFQdxVwN1qm3MkL37W8Nah7IhCQJfwc
Zlw4dXvOMBH2t1tcALplf9dfTSWv43dADzweftsw5B1SsajxyRJFKNowLSHaULyi
Mb0+zHOZVpd8kv7iQCyMN+H4Y4zyXOBLkYHGHhyz3dCjYHC9Sbpu/lzUT58/YzPe
HrmxrCKB8LvRFbl8JajuOgPGnVR4Q26BWSeNhDRUnzaKFIVBomh/MJRF60CYIZPk
lmUvpNArMbHqV2+mFglgMg1mezOoCzeQWYwVJOgrVXlRykqJ0KqxHlfhwntWL/0Q
ZH7YoootUT4YSzRjMqnzc+IUkQIDAQABoyMwITAOBgNVHQ8BAf8EBAMCAKQwDwYD
VR0TAQH/BAUwAwEB/zALBgkqhkiG9w0BAQsDggEBAICbUNAQRG2Y9p8sy2+7q8qn
RzmdOalFAp4g3nkBmvzp9mnwJK9ezTTq0oAGIcNHMK+7MnI8wnBXFHijtJpLWyCl
LOV7uj6fJafWGlEQ7nbnI78gsRGlN56MalqJEb3Jaa8eTOY9QH35wAmvyyECxYTI
e69X0GUWSYd8t0nayYZe9fIpJHh2x4brDqLuhizT2z4kMHuhwlChwYQuUQTkIeWP
ywoniSd90DMdyRuxXh+22lQAlHyDk6D9LMFZ7OEtYcwQeH26PFkJUIcxVTqjdpU7
ZMvmRe+fs3DIM2gz9bS1DVCEdE2UxPmqosaXxQY8InKSgTT2ExnB2/2mQ/hVq6M=
-----END CERTIFICATE-----`

		return []byte(pem), nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(pool.Subjects()) < 1 {
		t.Fatal("No subjects found")
	}

	for _, subj := range pool.Subjects() {
		if !strings.Contains(string(subj), "sthulb") {
			t.Fatal("Subject is wrong")
		}
	}
}
