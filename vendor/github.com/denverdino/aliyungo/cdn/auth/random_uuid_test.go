package auth

import (
	"testing"
)

const iterations = 1000

func TestUUID4Generation(t *testing.T) {
	for i := 0; i < iterations; i++ {
		u := GenerateUUID()

		if u[6]&0xf0 != 0x40 {
			t.Fatalf("version byte not correctly set: %v, %08b %08b", u, u[6], u[6]&0xf0)
		}

		if u[8]&0xc0 != 0x80 {
			t.Fatalf("top order 8th byte not correctly set: %v, %b", u, u[8])
		}
	}
}
