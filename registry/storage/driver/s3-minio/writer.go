package s3

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"sort"

	"github.com/minio/minio-go"

	"github.com/docker/distribution/context"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
)

const chunkSize = 5 << 20

type completedParts []minio.CompletePart

func (a completedParts) Len() int           { return len(a) }
func (a completedParts) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a completedParts) Less(i, j int) bool { return a[i].PartNumber < a[j].PartNumber }

type writerState int

const (
	writerOpen writerState = iota
	writerClosed
	writerCommitted
	writerCancelled
)

func (s writerState) Err() error {
	switch s {
	case writerOpen:
		return nil
	case writerClosed:
		return fmt.Errorf("writer is closed")
	case writerCommitted:
		return fmt.Errorf("writer is committed")
	case writerCancelled:
		return fmt.Errorf("writer is cancelled")
	}
	return fmt.Errorf("writer is in an unknown state")
}

type writerMeta struct {
	Generation int
	Size       int64
}

func writerMetaObject(path string) string {
	return path + ".tmp/meta"
}

func loadWriterMeta(ctx context.Context, d *driver, path string) (writerMeta, error) {
	var meta writerMeta

	data, err := d.GetContent(ctx, writerMetaObject(path))
	if err != nil {
		return meta, err
	}

	if err := json.Unmarshal(data, &meta); err != nil {
		return meta, fmt.Errorf("error unmarshalling writer metadata %s: %v", path, err)
	}
	return meta, nil
}

func (m writerMeta) store(ctx context.Context, d *driver, path string) error {
	data, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("error marshalling writer metadata %s: %v", path, err)
	}

	return d.PutContent(ctx, writerMetaObject(path), data)
}

type writer struct {
	driver   *driver
	chunker  Chunker
	path     string
	meta     writerMeta
	state    writerState
	uploadID string
	parts    []minio.ObjectPart
}

// objectPartWriter uploads a part to S3. On successful upload ObjectPart is
// appended to objectPartWriter.w.parts.
type objectPartWriter struct {
	w    *writer
	done chan struct{}
	part minio.ObjectPart
	err  error
	io.WriteCloser
}

// newObjectPartWriter returns a writer that uploads a part to S3. If less than
// chunkSize bytes are written, the part will be padded with zero bytes.
func newObjectPartWriter(w *writer, chunkSize int64) *objectPartWriter {
	opw := &objectPartWriter{
		w:    w,
		done: make(chan struct{}),
	}
	pipeReader, pipeWriter := io.Pipe()
	go func() {
		opw.part, opw.err = w.driver.S3.PutObjectPart(w.driver.Bucket, w.driver.s3Path(w.generationPath()), w.uploadID, len(w.parts)+1, chunkSize, ioutil.NopCloser(pipeReader), nil, nil)
		pipeReader.CloseWithError(opw.err)
		close(opw.done)
	}()
	opw.WriteCloser = &zeroPaddedWriter{
		W:    pipeWriter,
		Size: chunkSize,
	}
	return opw
}

func (opw *objectPartWriter) Close() error {
	err := opw.WriteCloser.Close()
	if err != nil {
		return err
	}
	<-opw.done
	if opw.err != nil {
		return fmt.Errorf("error uploading object part %d for %s: %v", len(opw.w.parts)+1, opw.w.path, opw.err)
	}
	opw.w.parts = append(opw.w.parts, opw.part)
	return nil
}

func newWriter(d *driver, path string, meta writerMeta) storagedriver.FileWriter {
	w := &writer{
		driver: d,
		path:   path,
		meta:   meta,
	}
	return w
}

func (w *writer) Size() int64 {
	return w.meta.Size
}

func (w *writer) Write(p []byte) (int, error) {
	if err := w.ensureUploadID(); err != nil {
		return 0, err
	}

	n, err := w.chunker.Write(p)
	w.meta.Size += int64(n)
	return n, err
}

func (w *writer) generationPath() string {
	return fmt.Sprintf("%s.tmp/gen-%d", w.path, w.meta.Generation)
}

func (w *writer) ensureUploadID() error {
	if w.uploadID != "" {
		return nil
	}

	w.meta.Generation++

	uploadID, err := w.driver.S3.NewMultipartUpload(w.driver.Bucket, w.driver.s3Path(w.generationPath()), map[string][]string{
		"Content-Type": {w.driver.getContentType()},
		"x-amz-acl":    {w.driver.getACL()},
	})
	if err != nil {
		w.meta.Generation--
		return err
	}

	w.uploadID = uploadID

	w.chunker = Chunker{
		Size: chunkSize,
		New: func() io.WriteCloser {
			return newObjectPartWriter(w, chunkSize)
		},
	}

	if w.meta.Generation > 1 {
		// FIXME: if the previous generation is less than 5 MB, then we must use client-side copy.
		// Otherwise we should be able to use server-side copy.
		return fmt.Errorf("TODO: copy data from the previous generation")
	}

	return nil
}

func (w *writer) abort() error {
	err := w.driver.S3.AbortMultipartUpload(w.driver.Bucket, w.driver.s3Path(w.generationPath()), w.uploadID)
	if err != nil {
		return fmt.Errorf("error aborting upload %s: %v", w.generationPath(), err)
	}

	w.uploadID = ""

	return nil
}

func (w *writer) complete(ctx context.Context) error {
	if w.uploadID == "" {
		return nil
	}

	if err := w.chunker.Close(); err != nil {
		return fmt.Errorf("error closing writer %s: %v", w.generationPath(), err)
	}

	var completedUploadedParts completedParts
	for _, part := range w.parts {
		completedUploadedParts = append(completedUploadedParts, minio.CompletePart{
			ETag:       part.ETag,
			PartNumber: part.PartNumber,
		})
	}

	sort.Sort(completedUploadedParts)

	err := w.driver.S3.CompleteMultipartUpload(w.driver.Bucket, w.driver.s3Path(w.generationPath()), w.uploadID, completedUploadedParts)
	if err != nil {
		// best effort cleanup
		_ = w.abort()
		return fmt.Errorf("error completing upload %s: %v", w.generationPath(), err)
	}

	w.uploadID = ""

	return w.meta.store(ctx, w.driver, w.path)

	// TODO: remove previous .gen object
}

func (w *writer) Close() error {
	if err := w.state.Err(); err != nil {
		return err
	}

	if err := w.complete(context.Background()); err != nil {
		return err
	}

	w.state = writerClosed

	return nil
}

func (w *writer) Cancel() error {
	if err := w.state.Err(); err != nil {
		return err
	}

	if err := w.abort(); err != nil {
		return err
	}

	if err := w.driver.Delete(context.Background(), w.path); err != nil {
		return fmt.Errorf("error deleting files after cancel: %v", err)
	}

	w.state = writerCancelled

	return nil
}

func (w *writer) Commit() error {
	if err := w.state.Err(); err != nil {
		return err
	}

	if err := w.complete(context.Background()); err != nil {
		return err
	}

	dst, err := minio.NewDestinationInfo(w.driver.Bucket, w.driver.s3Path(w.path), nil, nil)
	if err != nil {
		return err
	}

	src := minio.NewSourceInfo(w.driver.Bucket, w.driver.s3Path(w.generationPath()), nil)
	src.SetRange(0, w.meta.Size-1)

	if err := w.driver.S3.CopyObject(dst, src); err != nil {
		return fmt.Errorf("error copying object %v to %v: %v", src, dst, err)
	}

	w.state = writerCommitted

	if err := w.driver.Delete(context.Background(), w.path+".tmp"); err != nil {
		return fmt.Errorf("error deleting temporary files after commit: %v", err)
	}

	return nil
}

func createWriter(d *driver, path string) (storagedriver.FileWriter, error) {
	return newWriter(d, path, writerMeta{}), nil
}

func resumeWriter(ctx context.Context, d *driver, path string) (storagedriver.FileWriter, error) {
	meta, err := loadWriterMeta(ctx, d, path)
	if _, ok := err.(storagedriver.PathNotFoundError); ok {
		meta = writerMeta{}
	} else if err != nil {
		return nil, err
	}

	return newWriter(d, path, meta), nil
}
