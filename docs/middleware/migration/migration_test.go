package migration

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/docker/dhe-deploy/registry/middleware/mocks"
	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/registry/storage"
	"github.com/docker/distribution/registry/storage/cache/memory"
	"github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/inmemory"

	"github.com/stretchr/testify/mock"
)

const root = "/docker/registry/v2/"

type env struct {
	registry distribution.Namespace
	driver   driver.StorageDriver
	ctx      context.Context
}

func setupRegistry(t *testing.T) *env {
	d := inmemory.New()
	ctx := context.Background()
	registry, err := storage.NewRegistry(
		ctx,
		d,
		storage.BlobDescriptorCacheProvider(memory.NewInMemoryBlobDescriptorCacheProvider()),
		storage.EnableRedirect,
	)
	if err != nil {
		t.Fatalf("error iunstantiating registry: %v", err)
	}

	// Add data to registry
	var prefix = root + "repositories/admin/"
	data := map[string]interface{}{
		"content": map[string]string{
			// REPOSITORIES
			//a
			prefix + "a-repo/_layers/sha256/1f8d6e1edee77de035d79ca992df4e5cc8d358ec38f527077a84945a79907566/link":                     "sha256:1f8d6e1edee77de035d79ca992df4e5cc8d358ec38f527077a84945a79907566",
			prefix + "a-repo/_layers/sha256/6bf8e372a8396bbf22c0b2e0eebdad5ac3da97357621fe68de694bd4de23639d/link":                     "sha256:6bf8e372a8396bbf22c0b2e0eebdad5ac3da97357621fe68de694bd4de23639d",
			prefix + "a-repo/_manifests/revisions/sha256/1f8d6e1edee77de035d79ca992df4e5cc8d358ec38f527077a84945a79907566/link":        "sha256:1f8d6e1edee77de035d79ca992df4e5cc8d358ec38f527077a84945a79907566",
			prefix + "a-repo/_manifests/tags/a-tag/current/link":                                                                       "sha256:1f8d6e1edee77de035d79ca992df4e5cc8d358ec38f527077a84945a79907566",
			prefix + "a-repo/_manifests/tags/a-tag/index/sha256/1f8d6e1edee77de035d79ca992df4e5cc8d358ec38f527077a84945a79907566/link": "sha256:1f8d6e1edee77de035d79ca992df4e5cc8d358ec38f527077a84945a79907566",
			//b
			prefix + "b-repo/_layers/sha256/1f8d6e1edee77de035d79ca992df4e5cc8d358ec38f527077a84945a79907566/link":                     "sha256:1f8d6e1edee77de035d79ca992df4e5cc8d358ec38f527077a84945a79907566",
			prefix + "b-repo/_layers/sha256/6bf8e372a8396bbf22c0b2e0eebdad5ac3da97357621fe68de694bd4de23639d/link":                     "sha256:6bf8e372a8396bbf22c0b2e0eebdad5ac3da97357621fe68de694bd4de23639d",
			prefix + "b-repo/_manifests/revisions/sha256/1f8d6e1edee77de035d79ca992df4e5cc8d358ec38f527077a84945a79907566/link":        "sha256:1f8d6e1edee77de035d79ca992df4e5cc8d358ec38f527077a84945a79907566",
			prefix + "b-repo/_manifests/tags/b-tag/current/link":                                                                       "sha256:1f8d6e1edee77de035d79ca992df4e5cc8d358ec38f527077a84945a79907566",
			prefix + "b-repo/_manifests/tags/b-tag/index/sha256/1f8d6e1edee77de035d79ca992df4e5cc8d358ec38f527077a84945a79907566/link": "sha256:1f8d6e1edee77de035d79ca992df4e5cc8d358ec38f527077a84945a79907566",
			// MANIFESTS
			root + "blobs/sha256/1f/1f8d6e1edee77de035d79ca992df4e5cc8d358ec38f527077a84945a79907566/data": V2_MANIFEST_1,
			root + "blobs/sha256/6b/6bf8e372a8396bbf22c0b2e0eebdad5ac3da97357621fe68de694bd4de23639d/data": V2_MANIFEST_CONFIG_1,
		},
	}
	for path, blob := range data["content"].(map[string]string) {
		d.PutContent(ctx, path, []byte(blob))
	}

	return &env{
		registry,
		d,
		ctx,
	}
}

func TestMigrateResumes(t *testing.T) {
	env := setupRegistry(t)

	tests := []struct {
		migration     *migration
		expectedRepos []string
	}{
		{
			migration: &migration{
				reg:          env.registry,
				isFromResume: false,
			},
			expectedRepos: []string{"admin/a-repo", "admin/b-repo"},
		},
		{
			migration: &migration{
				reg:          env.registry,
				isFromResume: true,
				currentRepo:  "admin/b-repo",
			},
			expectedRepos: []string{"admin/b-repo"},
		},
	}

	for _, test := range tests {
		// Iterate through the repositories, storing each repository name within
		// iteratedRepos. We can then compare which repos were passed to onTagFunc
		// to check resumes
		iteratedRepos := []string{}
		onTagFunc := func(ctx context.Context, repo distribution.Repository, tagName string, tag distribution.Descriptor) error {
			iteratedRepos = append(iteratedRepos, repo.Named().Name())
			return nil
		}
		test.migration.enumerator = NewEnumerator(onTagFunc)
		if _, err := test.migration.Migrate(env.ctx); err != nil {
			t.Fatalf("error migrating: %s", err)
		}

		if !reflect.DeepEqual(iteratedRepos, test.expectedRepos) {
			t.Fatalf("resume failed, expected vs actual repo iteration: %s vs %s", test.expectedRepos, iteratedRepos)
		}
	}

}

// This is a basic test asserting that there are no obvious errors with
// the migration logic.
func TestAddTagAndManifest(t *testing.T) {
	env := setupRegistry(t)
	store := mocks.NewStore()
	migration := NewMigration(env.registry, store)

	store.TagStore.On(
		"PutTag",
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfTypeArgument("*storage.repository"),
		mock.AnythingOfType("string"),
		mock.AnythingOfType("distribution.Descriptor"),
	).Return(nil).Run(func(a mock.Arguments) {
		fmt.Printf("%#v", a)
	})

	store.ManifestStore.On(
		"PutManifest",
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfType("string"),
		mock.AnythingOfType("string"),
		mock.AnythingOfType("*schema2.DeserializedManifest"),
	).Return(nil).Run(func(a mock.Arguments) {
		fmt.Printf("%#v", a)
	})

	_, err := migration.Migrate(env.ctx)
	if err != nil {
		t.Fatalf("unexpected error during migration: %s", err)
	}
}

// Assert that failing during a migration returns no error
// and instead only logs the error
func TestAddTagAndManifestReturnsNil(t *testing.T) {
	env := setupRegistry(t)
	store := mocks.NewStore()
	migration := NewMigration(env.registry, store)

	// When we get admin/a-repo we can fail fast.
	store.TagStore.On(
		"PutTag",
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfTypeArgument("*storage.repository"),
		mock.AnythingOfType("string"),
		mock.AnythingOfType("distribution.Descriptor"),
	).Return(nil)

	store.ManifestStore.On(
		"PutManifest",
		mock.AnythingOfType("*context.valueCtx"),
		mock.AnythingOfType("string"),
		mock.AnythingOfType("string"),
		mock.AnythingOfType("*schema2.DeserializedManifest"),
	).Return(nil)

	_, err := migration.Migrate(env.ctx)
	if err != nil {
		t.Fatalf("unexpected error during migration: %v", err)
	}
}

const V2_MANIFEST_1 = `
{
	"schemaVersion": 2,
	"mediaType": "application/vnd.docker.distribution.manifest.v2+json",
	"config": {
		"mediaType": "application/vnd.docker.container.image.v1+json",
		"size": 1473,
		"digest": "sha256:6bf8e372a8396bbf22c0b2e0eebdad5ac3da97357621fe68de694bd4de23639d"
	},
	"layers": [
		{
			"mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
			"size": 146,
			"digest": "sha256:c170e8502f05562c30101cd65993e514cf63d242d6f14af6ca49896168c59ffd"
		}
	]
}
`

const V2_MANIFEST_CONFIG_1 = `
{
	"architecture": "amd64",
	"config": {
		"Hostname": "9aec87ce8e45",
		"Domainname": "",
		"User": "",
		"AttachStdin": false,
		"AttachStdout": false,
		"AttachStderr": false,
		"Tty": false,
		"OpenStdin": false,
		"StdinOnce": false,
		"Env": [
			"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
		],
		"Cmd": [
			"/true"
		],
		"Image": "sha256:bbadf13f1e9e0d1629c07ad1e7eedcc5a6383300b7701c131a6f0beac49866ad",
		"Volumes": null,
		"WorkingDir": "",
		"Entrypoint": null,
		"OnBuild": null,
		"Labels": {
		}
	},
	"container": "dab58e1226ef3b699c25b7befc7cec562707a959135d130f667a039e18e63f72",
	"container_config": {
		"Hostname": "9aec87ce8e45",
		"Domainname": "",
		"User": "",
		"AttachStdin": false,
		"AttachStdout": false,
		"AttachStderr": false,
		"Tty": false,
		"OpenStdin": false,
		"StdinOnce": false,
		"Env": [
			"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
		],
		"Cmd": [
			"/bin/sh",
			"-c",
			"#(nop) CMD [\"/true\"]"
		],
		"Image": "sha256:bbadf13f1e9e0d1629c07ad1e7eedcc5a6383300b7701c131a6f0beac49866ad",
		"Volumes": null,
		"WorkingDir": "",
		"Entrypoint": null,
		"OnBuild": null,
		"Labels": {
		}
	},
	"created": "2016-05-19T20:38:48.345518736Z",
	"docker_version": "1.11.1",
	"history": [
		{
			"created": "2016-05-19T20:38:48.277232795Z",
			"created_by": "/bin/sh -c #(nop) ADD file:513005a00bb6ce26c9eb571d6f16e0c12378ba40f8e3100bcb484db53008e3b2 in /true"
		},
		{
			"created": "2016-05-19T20:38:48.345518736Z",
			"created_by": "/bin/sh -c #(nop) CMD [\"/true\"]",
			"empty_layer": true
		}
	],
	"os": "linux",
	"rootfs": {
		"type": "layers",
		"diff_ids": [
			"sha256:af593d271f82964b57d51cc5e647c6076fb160bf8620f605848130110f0ed647"
		]
	}
}
`
