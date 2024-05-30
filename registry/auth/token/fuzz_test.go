package token

import (
	"testing"

	fuzz "github.com/AdaLogics/go-fuzz-headers"
	"github.com/go-jose/go-jose/v4"
)

func FuzzToken1(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		ff := fuzz.NewConsumer(data)
		rawToken, err := ff.GetString()
		if err != nil {
			return
		}
		verifyOps := VerifyOptions{}
		err = ff.GenerateStruct(&verifyOps)
		if err != nil {
			return
		}
		token, err := NewToken(rawToken, []jose.SignatureAlgorithm{jose.EdDSA, jose.RS384})
		if err != nil {
			return
		}
		_, err = token.Verify(verifyOps)
		if err != nil {
			return
		}
		_, _ = token.VerifySigningKey(verifyOps)
	})
}

func FuzzToken2(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		ff := fuzz.NewConsumer(data)
		verifyOps := VerifyOptions{}
		err := ff.GenerateStruct(&verifyOps)
		if err != nil {
			return
		}
		token := &Token{}
		err = ff.GenerateStruct(token)
		if err != nil {
			return
		}
		_, err = token.Verify(verifyOps)
		if err != nil {
			return
		}
		_, _ = token.VerifySigningKey(verifyOps)
	})
}
