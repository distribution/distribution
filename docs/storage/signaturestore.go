package storage

import (
	"path"

	"github.com/docker/distribution"
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

	var signatures [][]byte
	for _, sigPath := range signaturePaths {
		// Append the link portion
		sigPath = path.Join(sigPath, "link")

		// TODO(stevvooe): These fetches should be parallelized for performance.
		p, err := s.blobStore.linked(sigPath)
		if err != nil {
			return nil, err
		}

		signatures = append(signatures, p)
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
