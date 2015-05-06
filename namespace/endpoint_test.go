package namespace

import "testing"

func TestRemoteEndpoint(t *testing.T) {
	entries := mustEntries(`
docker.com/remote        push         https://registry.base.docker.com 5 version=2.0.1 trim name=production
docker.com/remote        pull         https://registry.base.docker.com 5 version=2.0.1 trim name=production
docker.com/remote        push         https://mirror.base.docker.com 10 version=2.0
docker.com/remote        pull         https://mirror.base.docker.com 10 version=2.0
docker.com/remote        pull         https://registry.base.docker.com version=1.0 trim
docker.com/remote        push         https://registry.base.docker.com version=1.0 trim
docker.com/remote        index        https://search.base.docker.com
docker.com/remote        index        search.docker.com
docker.com/remote/extend namespace    docker.com/remote
`)
	endpoints, err := GetRemoteEndpoints(entries)
	if err != nil {
		t.Fatalf("Error getting endpoints: %s", err)
	}

	if len(endpoints) != 8 {
		t.Fatalf("Unexpected number of endpoints: %d, expected 8", len(endpoints))
	}

	// TODO(dmcgowan): check each value
}
