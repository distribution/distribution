package testsuites

import (
	"bytes"
	"context"
	"io"
	"path"
	"testing"
)

type DriverBenchmarkSuite struct {
	Suite *DriverSuite
}

func NewDriverBenchmarkSuite(ds *DriverSuite) *DriverBenchmarkSuite {
	return &DriverBenchmarkSuite{Suite: ds}
}

// BenchmarkPutGetEmptyFiles benchmarks PutContent/GetContent for 0B files
func (s *DriverBenchmarkSuite) BenchmarkPutGetEmptyFiles(b *testing.B) {
	s.benchmarkPutGetFiles(b, 0)
}

// BenchmarkPutGet1KBFiles benchmarks PutContent/GetContent for 1KB files
func (s *DriverBenchmarkSuite) BenchmarkPutGet1KBFiles(b *testing.B) {
	s.benchmarkPutGetFiles(b, 1024)
}

// BenchmarkPutGet1MBFiles benchmarks PutContent/GetContent for 1MB files
func (s *DriverBenchmarkSuite) BenchmarkPutGet1MBFiles(b *testing.B) {
	s.benchmarkPutGetFiles(b, 1024*1024)
}

// BenchmarkPutGet1GBFiles benchmarks PutContent/GetContent for 1GB files
func (s *DriverBenchmarkSuite) BenchmarkPutGet1GBFiles(b *testing.B) {
	s.benchmarkPutGetFiles(b, 1024*1024*1024)
}

func (s *DriverBenchmarkSuite) benchmarkPutGetFiles(b *testing.B, size int64) {
	b.SetBytes(size)
	parentDir := randomPath(8)
	defer func() {
		b.StopTimer()
		// nolint:errcheck
		s.Suite.StorageDriver.Delete(s.Suite.ctx, firstPart(parentDir))
	}()

	for i := 0; i < b.N; i++ {
		filename := path.Join(parentDir, randomPath(32))
		err := s.Suite.StorageDriver.PutContent(s.Suite.ctx, filename, randomContents(size))
		s.Suite.Require().NoError(err)

		_, err = s.Suite.StorageDriver.GetContent(s.Suite.ctx, filename)
		s.Suite.Require().NoError(err)
	}
}

// BenchmarkStreamEmptyFiles benchmarks Writer/Reader for 0B files
func (s *DriverBenchmarkSuite) BenchmarkStreamEmptyFiles(b *testing.B) {
	s.benchmarkStreamFiles(b, 0)
}

// BenchmarkStream1KBFiles benchmarks Writer/Reader for 1KB files
func (s *DriverBenchmarkSuite) BenchmarkStream1KBFiles(b *testing.B) {
	s.benchmarkStreamFiles(b, 1024)
}

// BenchmarkStream1MBFiles benchmarks Writer/Reader for 1MB files
func (s *DriverBenchmarkSuite) BenchmarkStream1MBFiles(b *testing.B) {
	s.benchmarkStreamFiles(b, 1024*1024)
}

// BenchmarkStream1GBFiles benchmarks Writer/Reader for 1GB files
func (s *DriverBenchmarkSuite) BenchmarkStream1GBFiles(b *testing.B) {
	s.benchmarkStreamFiles(b, 1024*1024*1024)
}

func (s *DriverBenchmarkSuite) benchmarkStreamFiles(b *testing.B, size int64) {
	b.SetBytes(size)
	parentDir := randomPath(8)
	defer func() {
		b.StopTimer()
		// nolint:errcheck
		s.Suite.StorageDriver.Delete(s.Suite.ctx, firstPart(parentDir))
	}()

	for i := 0; i < b.N; i++ {
		filename := path.Join(parentDir, randomPath(32))
		writer, err := s.Suite.StorageDriver.Writer(s.Suite.ctx, filename, false)
		s.Suite.Require().NoError(err)
		written, err := io.Copy(writer, bytes.NewReader(randomContents(size)))
		s.Suite.Require().NoError(err)
		s.Suite.Require().Equal(size, written)

		err = writer.Commit(context.Background())
		s.Suite.Require().NoError(err)
		err = writer.Close()
		s.Suite.Require().NoError(err)

		rc, err := s.Suite.StorageDriver.Reader(s.Suite.ctx, filename, 0)
		s.Suite.Require().NoError(err)
		rc.Close()
	}
}

// BenchmarkList5Files benchmarks List for 5 small files
func (s *DriverBenchmarkSuite) BenchmarkList5Files(b *testing.B) {
	s.benchmarkListFiles(b, 5)
}

// BenchmarkList50Files benchmarks List for 50 small files
func (s *DriverBenchmarkSuite) BenchmarkList50Files(b *testing.B) {
	s.benchmarkListFiles(b, 50)
}

func (s *DriverBenchmarkSuite) benchmarkListFiles(b *testing.B, numFiles int64) {
	parentDir := randomPath(8)
	defer func() {
		b.StopTimer()
		// nolint:errcheck
		s.Suite.StorageDriver.Delete(s.Suite.ctx, firstPart(parentDir))
	}()

	for i := int64(0); i < numFiles; i++ {
		err := s.Suite.StorageDriver.PutContent(s.Suite.ctx, path.Join(parentDir, randomPath(32)), nil)
		s.Suite.Require().NoError(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		files, err := s.Suite.StorageDriver.List(s.Suite.ctx, parentDir)
		s.Suite.Require().NoError(err)
		s.Suite.Require().Equal(numFiles, int64(len(files)))
	}
}

// BenchmarkDelete5Files benchmarks Delete for 5 small files
func (s *DriverBenchmarkSuite) BenchmarkDelete5Files(b *testing.B) {
	s.benchmarkDeleteFiles(b, 5)
}

// BenchmarkDelete50Files benchmarks Delete for 50 small files
func (s *DriverBenchmarkSuite) BenchmarkDelete50Files(b *testing.B) {
	s.benchmarkDeleteFiles(b, 50)
}

func (s *DriverBenchmarkSuite) benchmarkDeleteFiles(b *testing.B, numFiles int64) {
	for i := 0; i < b.N; i++ {
		parentDir := randomPath(8)
		defer s.Suite.deletePath(firstPart(parentDir))

		b.StopTimer()
		for j := int64(0); j < numFiles; j++ {
			err := s.Suite.StorageDriver.PutContent(s.Suite.ctx, path.Join(parentDir, randomPath(32)), nil)
			s.Suite.Require().NoError(err)
		}
		b.StartTimer()

		// This is the operation we're benchmarking
		err := s.Suite.StorageDriver.Delete(s.Suite.ctx, firstPart(parentDir))
		s.Suite.Require().NoError(err)
	}
}
