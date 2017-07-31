package s3

import "io"

// Chunker writes data in chunks. Each Size bytes Chunker closes a chunk and
// starts a new one.
type Chunker struct {
	Size int
	New  func() io.WriteCloser

	w io.WriteCloser
	n int
}

func (c *Chunker) ensureWriter() error {
	if c.n == c.Size {
		err := c.w.Close()
		if err != nil {
			return err
		}
		c.w = nil
		c.n = 0
	}

	if c.w == nil {
		c.w = c.New()
		c.n = 0
	}

	return nil
}

func (c *Chunker) Write(buf []byte) (int, error) {
	if len(buf) == 0 {
		return 0, nil
	}

	written := 0
	for len(buf) > 0 {
		err := c.ensureWriter()
		if err != nil {
			return written, err
		}

		b := buf
		if len(b) > c.Size-c.n {
			b = buf[:c.Size-c.n]
		}
		n, err := c.w.Write(b)
		buf = buf[n:]
		written += n
		c.n += n
		if err != nil {
			return written, err
		}
	}
	return written, nil
}

// Close closes the last created chunk, if any.
func (c *Chunker) Close() error {
	if c.w != nil {
		err := c.w.Close()
		c.w = nil
		c.n = 0
		return err
	}
	return nil
}
