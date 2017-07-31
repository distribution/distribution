package s3

import "io"

var zeroBuffer [32 << 10]byte

type zeroPaddedWriter struct {
	Size int64
	W    io.WriteCloser
	n    int64
}

func (w *zeroPaddedWriter) Write(buf []byte) (int, error) {
	n, err := w.W.Write(buf)
	w.n += int64(n)
	return n, err
}

func (w *zeroPaddedWriter) Close() error {
	for w.n < w.Size {
		pad := w.Size - w.n
		if pad > int64(len(zeroBuffer)) {
			pad = int64(len(zeroBuffer))
		}
		_, err := w.Write(zeroBuffer[:pad])
		if err != nil {
			return err
		}
	}
	return w.W.Close()
}
