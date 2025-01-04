package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/distribution/distribution/v3"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

type httpBlobUpload struct {
	ctx context.Context

	statter distribution.BlobStatter
	client  *http.Client

	uuid      string
	startedAt time.Time

	location string // always the last value of the location header.
	offset   int64
	closed   bool
}

func (hbu *httpBlobUpload) Reader() (io.ReadCloser, error) {
	panic("Not implemented")
}

func (hbu *httpBlobUpload) handleErrorResponse(resp *http.Response) error {
	if resp.StatusCode == http.StatusNotFound {
		return distribution.ErrBlobUploadUnknown
	}
	return HandleHTTPResponseError(resp)
}

func (hbu *httpBlobUpload) ReadFrom(r io.Reader) (n int64, err error) {
	req, err := http.NewRequestWithContext(hbu.ctx, http.MethodPatch, hbu.location, io.NopCloser(r))
	if err != nil {
		return 0, err
	}
	defer req.Body.Close()

	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := hbu.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if err := hbu.handleErrorResponse(resp); err != nil {
		return 0, err
	}

	hbu.uuid = resp.Header.Get("Docker-Upload-UUID")
	hbu.location, err = sanitizeLocation(resp.Header.Get("Location"), hbu.location)
	if err != nil {
		return 0, err
	}
	rng := resp.Header.Get("Range")
	var start, end int64
	if n, err := fmt.Sscanf(rng, "%d-%d", &start, &end); err != nil {
		return 0, err
	} else if n != 2 || end < start {
		return 0, fmt.Errorf("bad range format: %s", rng)
	}

	hbu.offset += end - start + 1
	return (end - start + 1), nil
}

func (hbu *httpBlobUpload) Write(p []byte) (n int, err error) {
	req, err := http.NewRequestWithContext(hbu.ctx, http.MethodPatch, hbu.location, bytes.NewReader(p))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Range", fmt.Sprintf("%d-%d", hbu.offset, hbu.offset+int64(len(p)-1)))
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(p)))
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := hbu.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if err := hbu.handleErrorResponse(resp); err != nil {
		return 0, err
	}

	hbu.uuid = resp.Header.Get("Docker-Upload-UUID")
	hbu.location, err = sanitizeLocation(resp.Header.Get("Location"), hbu.location)
	if err != nil {
		return 0, err
	}
	rng := resp.Header.Get("Range")
	var start, end int
	if n, err := fmt.Sscanf(rng, "%d-%d", &start, &end); err != nil {
		return 0, err
	} else if n != 2 || end < start {
		return 0, fmt.Errorf("bad range format: %s", rng)
	}

	hbu.offset += int64(end - start + 1)
	return (end - start + 1), nil
}

func (hbu *httpBlobUpload) Size() int64 {
	return hbu.offset
}

func (hbu *httpBlobUpload) ID() string {
	return hbu.uuid
}

func (hbu *httpBlobUpload) StartedAt() time.Time {
	return hbu.startedAt
}

func (hbu *httpBlobUpload) Commit(ctx context.Context, desc v1.Descriptor) (v1.Descriptor, error) {
	// TODO(dmcgowan): Check if already finished, if so just fetch
	req, err := http.NewRequestWithContext(hbu.ctx, http.MethodPut, hbu.location, nil)
	if err != nil {
		return v1.Descriptor{}, err
	}

	values := req.URL.Query()
	values.Set("digest", desc.Digest.String())
	req.URL.RawQuery = values.Encode()

	resp, err := hbu.client.Do(req)
	if err != nil {
		return v1.Descriptor{}, err
	}
	defer resp.Body.Close()

	if err := hbu.handleErrorResponse(resp); err != nil {
		return v1.Descriptor{}, err
	}

	return hbu.statter.Stat(ctx, desc.Digest)
}

func (hbu *httpBlobUpload) Cancel(ctx context.Context) error {
	req, err := http.NewRequestWithContext(hbu.ctx, http.MethodDelete, hbu.location, nil)
	if err != nil {
		return err
	}
	resp, err := hbu.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	return hbu.handleErrorResponse(resp)
}

func (hbu *httpBlobUpload) Close() error {
	hbu.closed = true
	return nil
}
