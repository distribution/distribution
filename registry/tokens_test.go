package registry

import (
	"testing"

	"github.com/docker/distribution/storage"
)

var layerUploadStates = []storage.LayerUploadState{
	{
		Name:   "hello",
		UUID:   "abcd-1234-qwer-0987",
		Offset: 0,
	},
	{
		Name:   "hello-world",
		UUID:   "abcd-1234-qwer-0987",
		Offset: 0,
	},
	{
		Name:   "h3ll0_w0rld",
		UUID:   "abcd-1234-qwer-0987",
		Offset: 1337,
	},
	{
		Name:   "ABCDEFG",
		UUID:   "ABCD-1234-QWER-0987",
		Offset: 1234567890,
	},
	{
		Name:   "this-is-A-sort-of-Long-name-for-Testing",
		UUID:   "dead-1234-beef-0987",
		Offset: 8675309,
	},
}

var secrets = []string{
	"supersecret",
	"12345",
	"a",
	"SuperSecret",
	"Sup3r... S3cr3t!",
	"This is a reasonably long secret key that is used for the purpose of testing.",
	"\u2603+\u2744", // snowman+snowflake
}

// TestLayerUploadTokens constructs stateTokens from LayerUploadStates and
// validates that the tokens can be used to reconstruct the proper upload state.
func TestLayerUploadTokens(t *testing.T) {
	tokenProvider := newHMACTokenProvider("supersecret")

	for _, testcase := range layerUploadStates {
		token, err := tokenProvider.layerUploadStateToToken(testcase)
		if err != nil {
			t.Fatal(err)
		}

		lus, err := tokenProvider.layerUploadStateFromToken(token)
		if err != nil {
			t.Fatal(err)
		}

		assertLayerUploadStateEquals(t, testcase, lus)
	}
}

// TestHMACValidate ensures that any HMAC token providers are compatible if and
// only if they share the same secret.
func TestHMACValidation(t *testing.T) {
	for _, secret := range secrets {
		tokenProvider1 := newHMACTokenProvider(secret)
		tokenProvider2 := newHMACTokenProvider(secret)
		badTokenProvider := newHMACTokenProvider("DifferentSecret")

		for _, testcase := range layerUploadStates {
			token, err := tokenProvider1.layerUploadStateToToken(testcase)
			if err != nil {
				t.Fatal(err)
			}

			lus, err := tokenProvider2.layerUploadStateFromToken(token)
			if err != nil {
				t.Fatal(err)
			}

			assertLayerUploadStateEquals(t, testcase, lus)

			_, err = badTokenProvider.layerUploadStateFromToken(token)
			if err == nil {
				t.Fatalf("Expected token provider to fail at retrieving state from token: %s", token)
			}

			badToken, err := badTokenProvider.layerUploadStateToToken(testcase)
			if err != nil {
				t.Fatal(err)
			}

			_, err = tokenProvider1.layerUploadStateFromToken(badToken)
			if err == nil {
				t.Fatalf("Expected token provider to fail at retrieving state from token: %s", badToken)
			}

			_, err = tokenProvider2.layerUploadStateFromToken(badToken)
			if err == nil {
				t.Fatalf("Expected token provider to fail at retrieving state from token: %s", badToken)
			}
		}
	}
}

func assertLayerUploadStateEquals(t *testing.T, expected storage.LayerUploadState, received storage.LayerUploadState) {
	if expected.Name != received.Name {
		t.Fatalf("Expected Name=%q, Received Name=%q", expected.Name, received.Name)
	}
	if expected.UUID != received.UUID {
		t.Fatalf("Expected UUID=%q, Received UUID=%q", expected.UUID, received.UUID)
	}
	if expected.Offset != received.Offset {
		t.Fatalf("Expected Offset=%d, Received Offset=%d", expected.Offset, received.Offset)
	}
}
