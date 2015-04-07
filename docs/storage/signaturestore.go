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
	signatures := make([][]byte, len(signaturePaths)) // make space for everything
	errCh := make(chan error, 1)                      // buffered chan so one proceeds
	for i, sigPath := range signaturePaths {
		// Append the link portion
		sigPath = path.Join(sigPath, "link")

		wg.Add(1)
		go func(idx int, sigPath string) {
			defer wg.Done()
			context.GetLogger(s.ctx).
				Debugf("fetching signature from %q", sigPath)
			p, err := s.blobStore.linked(sigPath)
			if err != nil {
				context.GetLogger(s.ctx).
					Errorf("error fetching signature from %q: %v", sigPath, err)

				// try to send an error, if it hasn't already been sent.
				select {
				case errCh <- err:
				default:
				}

				return
			}
			signatures[idx] = p
		}(i, sigPath)
	}
	wg.Wait()

	select {
	case err := <-errCh:
		// just return the first error, similar to single threaded code.
		return nil, err
	default:
		// pass
	}

	return signatures, nil
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
