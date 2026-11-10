package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
)

func BenchmarkFileReader(b *testing.B) {
	for _, size := range []int{4 * 1024, 16 * 1024 * 1024} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			ctx := context.Background()
			driver := inmemory.New()
			if err := driver.PutContent(ctx, "/blob", bytes.Repeat([]byte("x"), size)); err != nil {
				b.Fatal(err)
			}
			b.SetBytes(int64(size))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				fr, err := newFileReader(ctx, driver, "/blob", int64(size))
				if err != nil {
					b.Fatal(err)
				}
				n, err := io.Copy(io.Discard, fr)
				fr.Close()
				if err != nil || n != int64(size) {
					b.Fatalf("read %d bytes: %v", n, err)
				}
			}
		})
	}
}
