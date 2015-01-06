package testutil

import (
	"archive/tar"
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"io/ioutil"
	mrand "math/rand"
	"time"

	"github.com/docker/docker/pkg/tarsum"
)

// CreateRandomTarFile creates a random tarfile, returning it as an
// io.ReadSeeker along with its tarsum. An error is returned if there is a
// problem generating valid content.
func CreateRandomTarFile() (rs io.ReadSeeker, tarSum string, err error) {
	nFiles := mrand.Intn(10) + 10
	target := &bytes.Buffer{}
	wr := tar.NewWriter(target)

	// Perturb this on each iteration of the loop below.
	header := &tar.Header{
		Mode:       0644,
		ModTime:    time.Now(),
		Typeflag:   tar.TypeReg,
		Uname:      "randocalrissian",
		Gname:      "cloudcity",
		AccessTime: time.Now(),
		ChangeTime: time.Now(),
	}

	for fileNumber := 0; fileNumber < nFiles; fileNumber++ {
		fileSize := mrand.Int63n(1<<20) + 1<<20

		header.Name = fmt.Sprint(fileNumber)
		header.Size = fileSize

		if err := wr.WriteHeader(header); err != nil {
			return nil, "", err
		}

		randomData := make([]byte, fileSize)

		// Fill up the buffer with some random data.
		n, err := rand.Read(randomData)

		if n != len(randomData) {
			return nil, "", fmt.Errorf("short read creating random reader: %v bytes != %v bytes", n, len(randomData))
		}

		if err != nil {
			return nil, "", err
		}

		nn, err := io.Copy(wr, bytes.NewReader(randomData))
		if nn != fileSize {
			return nil, "", fmt.Errorf("short copy writing random file to tar")
		}

		if err != nil {
			return nil, "", err
		}

		if err := wr.Flush(); err != nil {
			return nil, "", err
		}
	}

	if err := wr.Close(); err != nil {
		return nil, "", err
	}

	reader := bytes.NewReader(target.Bytes())

	// A tar builder that supports tarsum inline calculation would be awesome
	// here.
	ts, err := tarsum.NewTarSum(reader, true, tarsum.Version1)
	if err != nil {
		return nil, "", err
	}

	nn, err := io.Copy(ioutil.Discard, ts)
	if nn != int64(len(target.Bytes())) {
		return nil, "", fmt.Errorf("short copy when getting tarsum of random layer: %v != %v", nn, len(target.Bytes()))
	}

	if err != nil {
		return nil, "", err
	}

	return bytes.NewReader(target.Bytes()), ts.Sum(nil), nil
}
