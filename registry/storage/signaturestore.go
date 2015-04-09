package storage

import (
	"path"
	"sync"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
)

type signatureStore struct {
	*repository
}

var _ distribution.SignatureService = &signatureStore{}

func (s *signatureStore) Get(dgst digest.Digest) ([][]byte, error) {
	signaturesPath, err := s.pm.path(manifestSignaturesPathSpec{
		name:     s.Name(),
		revision: dgst,
	})

	if err != nil {
		return nil, err
	}

	// Need to append signature digest algorithm to path to get all items.
	// Perhaps, this should be in the pathMapper but it feels awkward. This
	// can be eliminated by implementing listAll on drivers.
	signaturesPath = path.Join(signaturesPath, "sha256")

	signaturePaths, err := s.driver.List(signaturesPath)
	if err != nil {
		return nil, err
	}

	var wg sync.WaitGroup
	type result struct {
		index     int
		signature []byte
		err       error
	}
	ch := make(chan result)

	for i, sigPath := range signaturePaths {
		// Append the link portion
		sigPath = path.Join(sigPath, "link")

		wg.Add(1)
		go func(idx int, sigPath string) {
			defer wg.Done()
			context.GetLogger(s.ctx).
				Debugf("fetching signature from %q", sigPath)

			r := result{index: idx}
			if p, err := s.blobStore.linked(sigPath); err != nil {
				context.GetLogger(s.ctx).
					Errorf("error fetching signature from %q: %v", sigPath, err)
				r.err = err
			} else {
				r.signature = p
			}

			ch <- r
		}(i, sigPath)
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	// aggregrate the results
	signatures := make([][]byte, len(signaturePaths))
loop:
	for {
		select {
		case result := <-ch:
			signatures[result.index] = result.signature
			if result.err != nil && err == nil {
				// only set the first one.
				err = result.err
			}
		case <-done:
			break loop
		}
	}

	return signatures, err
}

func (s *signatureStore) Put(dgst digest.Digest, signatures ...[]byte) error {
	for _, signature := range signatures {
		signatureDigest, err := s.blobStore.put(signature)
		if err != nil {
			return err
		}

		signaturePath, err := s.pm.path(manifestSignatureLinkPathSpec{
			name:      s.Name(),
			revision:  dgst,
			signature: signatureDigest,
		})

		if err != nil {
			return err
		}

		if err := s.blobStore.link(signaturePath, signatureDigest); err != nil {
			return err
		}
	}
	return nil
}
