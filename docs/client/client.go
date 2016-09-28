package client

import (
	"crypto"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/docker/dhe-deploy/garant/authn"
	"github.com/docker/dhe-deploy/garant/authz"
	"github.com/docker/dhe-deploy/hubconfig"
	"github.com/docker/dhe-deploy/manager/schema"
	"github.com/docker/dhe-deploy/registry/middleware"
	middlewareErrors "github.com/docker/dhe-deploy/registry/middleware/errors"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/reference"
	"github.com/docker/distribution/registry/storage"
	"github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/factory"
	// all storage drivers
	_ "github.com/docker/distribution/registry/storage/driver/azure"
	_ "github.com/docker/distribution/registry/storage/driver/filesystem"
	_ "github.com/docker/distribution/registry/storage/driver/gcs"
	_ "github.com/docker/distribution/registry/storage/driver/inmemory"
	_ "github.com/docker/distribution/registry/storage/driver/middleware/cloudfront"
	_ "github.com/docker/distribution/registry/storage/driver/oss"
	_ "github.com/docker/distribution/registry/storage/driver/s3-aws"
	_ "github.com/docker/distribution/registry/storage/driver/swift"

	"github.com/docker/garant/auth"
	"github.com/palantir/stacktrace"
)

// RegistryClient defines all methods for DTR<>Registry API support
type RegistryClient interface {
	// DeleteRepository deletes an entire repository
	DeleteRepository(named string, r *schema.Repository) error

	// DeleteTag removes a tag from a named repository
	DeleteTag(named, tag string) error

	// DeleteManifest removes a manifest from a named repository
	DeleteManifest(named, digest string) error

	// CreateJWT creates a jwt representing valid authn and authz for registry actions
	// on behalf of a user
	CreateJWT(user *authn.User, repo, accessLevel string) (string, error)
}

// Client is a concrete implementation of RegistryClient
type client struct {
	// settings allows us to load DTR and registry settings from the store
	settings hubconfig.SettingsReader
	// driver is a concrete StorageDriver for registry blobstore ops
	driver driver.StorageDriver
	// store is a middleware.Store implementation, saving tag info in A DB
	store middleware.Store
	// repoManager is used when deleting repos
	repoManager *schema.RepositoryManager
	// ctx represents a context used in initialization
	ctx context.Context
}

// Opts is an exported struct representing options for instantiating a new
// client
type Opts struct {
	Settings    hubconfig.SettingsReader
	Store       middleware.Store
	RepoManager *schema.RepositoryManager
}

// Returns a new `client` type with the given configuration. A storage driver
// will also be instantiated from the configuration supplied.
func NewClient(ctx context.Context, opts Opts) (RegistryClient, error) {
	config, err := opts.Settings.RegistryConfig()
	if err != nil {
		return nil, stacktrace.Propagate(err, "error fetching registry config")
	}

	// FUCK THIS SHITTY HACK THIS SHOULD NEVER HAVE BEEN ALLOWED TO EXIST
	// whoever made this deserves a little seeing to. this is a copypasta
	if config.Storage.Type() == "filesystem" {
		params := config.Storage["filesystem"]
		params["rootdirectory"] = "/storage"
		config.Storage["filesystem"] = params
	}

	driver, err := factory.Create(config.Storage.Type(), config.Storage.Parameters())
	if err != nil {
		return nil, stacktrace.Propagate(err, "error creating distribution storage driver")
	}

	return &client{
		ctx:         ctx,
		settings:    opts.Settings,
		store:       opts.Store,
		repoManager: opts.RepoManager,
		driver:      driver,
	}, nil
}

// DeleteRepository removes an entire repository and all artifacts from DTR.
// To do this we need to remove all repository blobs, all tags from the
// metadata store and the repository from the DTR DB.
//
// In order to keep as consistent as possible with the blobstore the current
// strategy is:
//
// 1. Nuke the entire repo/name directory within blobstore
// 2. Wait for this to happen
// 3. Delete all tags from the database
//
// Note that this does not use the registry client directly; there is no way
// of deleting repositories within the API, plus repositories are created
// within the DTR DB directly.
//
// NOTE: the arguments for this are ridiculous because in order to delete
//       a repository we need to:
//       1. Query for the repository namespace to load it's UUID
//       2. Use the namespace UUID to generate the repo's PK (it's part of the
//          hash)
//       3. Query for the repository by the generated PK for the repo's UUID
//       4. Use THAT UUID to finally delete the repository.
// TO simplify this we're using arguments from the adminserver's filters.
//
// XXX: (tonyhb) After this has finished schedule a new job for consistency
// checking this repository. TODO: Define how the consistency checker
// guarantees consistency.
//
// XXX: Two-phase commit for deletes would be nice. In this case we'd need to
// delete from the blobstore, then delete from the database. If the database
// delete failed add a job to remove from the database to keep consistency.
// We currently have no notion of failed DB writes to retry later; this needs
// to be added for proper two phase commit.
func (c client) DeleteRepository(named string, r *schema.Repository) (err error) {
	// Do this first as it's non-destructive.
	repo, err := c.getRepo(named)
	if err != nil {
		return stacktrace.Propagate(err, "error instantiating distribution.Repository")
	}

	// Then look up all tags; this is a prerequisite and should be done before
	// destructive actions.
	tags, err := c.store.AllTags(c.ctx, repo)
	if err != nil {
		return stacktrace.Propagate(err, "error fetching tags for repository")
	}

	vacuum := storage.NewVacuum(context.Background(), c.driver)
	if err = vacuum.RemoveRepository(named); err != nil {
		// If this is an ErrPathNotFound error from distribution we can ignore;
		// the path is only made when a tag is pushed, and this repository
		// may have no tags.
		if _, ok := err.(driver.PathNotFoundError); !ok {
			return stacktrace.Propagate(err, "error removing repository from blobstore")
		}
	}

	// If one tag fails we should carry on deleting the remaining tags, returning
	// errors at the end of enumeration. This may produce more errors but should
	// have closer consistency to the blobstore.
	var errors = map[string]error{}
	for _, tag := range tags {
		if err := c.store.DeleteTag(c.ctx, repo, tag); err != nil {
			errors[tag] = err
		}
	}
	if len(errors) > 0 {
		return stacktrace.NewError("errors deleting tags from metadata store: %s", errors)
	}

	// Delete the repo from rethinkdb. See function notes above for info.
	if err := c.repoManager.DeleteRepositoryByPK(r.PK); err != nil {
		return stacktrace.Propagate(err, "unable to delete repo from database")
	}

	return nil
}

// DeleteTag attempts to delete a tag from the blobstore and metadata store.
//
// This is done by first deleting from the database using middleware.Store,
// then the blobstore using the storage.Repository
//
// If this is the last tag to reference a manifest the manifest will be left valid
// and in an undeleted state (ie. dangling). The GC should collect and delete
// dangling manifests.
func (c client) DeleteTag(named, tag string) error {
	repo, err := c.getRepo(named)
	if err != nil {
		return stacktrace.Propagate(err, "")
	}

	// Delete from the tagstore first; this is our primary source of truth and
	// should always be in a consistent state.
	if err := c.store.DeleteTag(c.ctx, repo, tag); err != nil && err != middlewareErrors.ErrNotFound {
		return stacktrace.Propagate(err, "error deleting tag from metadata store")
	}

	// getRepo returns a repository constructed from storage; calling Untag
	// on this TagService will remove the tag from the blobstore.
	if err := repo.Tags(c.ctx).Untag(c.ctx, tag); err != nil {
		// If this is an ErrPathNotFound error from distribution we can ignore;
		// the path is only made when a tag is pushed, and this repository
		// may have no tags.
		if _, ok := err.(driver.PathNotFoundError); !ok {
			return stacktrace.Propagate(err, "error deleting tag from blobstore")
		}
	}

	return nil
}

// DeleteManifest attempts to delete a manifest from the blobstore and metadata
// store.
//
// This is done by first deleting from the database using middleware.Store,
// then the blobstore using the storage.Repository
//
// This does not delete any tags pointing to this manifest. Instead, when the
// metadata store loads tags it checks to ensure the manifest it refers to is
// valid.
func (c client) DeleteManifest(named, dgst string) error {
	repo, err := c.getRepo(named)
	if err != nil {
		return stacktrace.Propagate(err, "")
	}

	mfstSrvc, err := repo.Manifests(c.ctx)
	if err != nil {
		return stacktrace.Propagate(err, "")
	}

	// Delete from the tagstore first; this is our primary source of truth and
	// should always be in a consistent state.
	err = c.store.DeleteManifest(c.ctx, named+"@"+dgst)
	if err != nil && err != middlewareErrors.ErrNotFound {
		return stacktrace.Propagate(err, "error deleting manifest from metadata store")
	}

	if err = mfstSrvc.Delete(c.ctx, digest.Digest(dgst)); err != nil {
		if _, ok := err.(driver.PathNotFoundError); !ok {
			return stacktrace.Propagate(err, "error deleting manifest from blobstore")
		}
	}

	return nil
}

// getRepo is a utility function which returns a distribution.Repository for a
// given repository name string
func (c client) getRepo(named string) (distribution.Repository, error) {
	// Note that this has no options enabled such as disabling v1 signatures or
	// middleware. It will ONLY perform operations using the blobstore storage
	// driver.
	reg, err := storage.NewRegistry(c.ctx, c.driver, storage.EnableDelete)
	if err != nil {
		return nil, stacktrace.Propagate(err, "error instantiating registry instance for deleting tags")
	}

	repoName, err := reference.WithName(named)
	if err != nil {
		return nil, stacktrace.Propagate(err, "error parsing repository name")
	}

	repo, err := reg.Repository(c.ctx, repoName)
	if err != nil {
		return nil, stacktrace.Propagate(err, "error constructing repository")
	}

	return repo, nil
}

// CreateJWT creates a jwt representing valid authn and authz for registry actions
// on behalf of a user
func (c client) CreateJWT(user *authn.User, repo, accessLevel string) (string, error) {
	// We need the DTR config and garant token signing key to generate a valid "iss" and
	// "aud" claim and sign the JWT correctly.
	uhc, err := c.settings.UserHubConfig()
	if err != nil {
		return "", stacktrace.Propagate(err, "error getting dtr config")
	}
	key, err := c.settings.GarantSigningKey()
	if err != nil {
		return "", stacktrace.Propagate(err, "error getting token signing key")
	}

	// service is our domain name which represents the "iss" and "aud" claims
	service := uhc.DTRHost

	var actions []string
	accessScopeSet := authz.AccessLevelScopeSets[accessLevel]
	for action := range accessScopeSet {
		actions = append(actions, action)
	}
	accessEntries := []accessEntry{
		{
			Resource: auth.Resource{
				Type: "repository",
				Name: repo,
			},
			Actions: actions,
		},
	}

	// Create a random string for a JTI claim. Garant doesn't yet record JTIs
	// to prevent replay attacks in DTR; we should.
	// TODO(tonyhb): record JTI claims from garant and prevent replay attacks
	byt := make([]byte, 15)
	io.ReadFull(rand.Reader, byt)
	jti := base64.URLEncoding.EncodeToString(byt)

	now := time.Now()

	joseHeader := map[string]interface{}{
		"typ": "JWT",
		"alg": "ES256",
	}

	if x5c := key.GetExtendedField("x5c"); x5c != nil {
		joseHeader["x5c"] = x5c
	} else {
		joseHeader["jwk"] = key.PublicKey()
	}

	var subject string
	if user != nil {
		subject = user.Account.Name
	}

	claimSet := map[string]interface{}{
		"iss":    service,
		"sub":    subject,
		"aud":    service,
		"exp":    now.Add(5 * time.Minute).Unix(),
		"nbf":    now.Unix(),
		"iat":    now.Unix(),
		"jti":    jti,
		"access": accessEntries,
	}

	var (
		joseHeaderBytes, claimSetBytes []byte
	)

	if joseHeaderBytes, err = json.Marshal(joseHeader); err != nil {
		return "", stacktrace.Propagate(err, "error encoding jose header")
	}
	if claimSetBytes, err = json.Marshal(claimSet); err != nil {
		return "", stacktrace.Propagate(err, "error encoding jwt claimset")
	}

	encodedJoseHeader := joseBase64Encode(joseHeaderBytes)
	encodedClaimSet := joseBase64Encode(claimSetBytes)
	encodingToSign := fmt.Sprintf("%s.%s", encodedJoseHeader, encodedClaimSet)

	var signatureBytes []byte
	if signatureBytes, _, err = key.Sign(strings.NewReader(encodingToSign), crypto.SHA256); err != nil {
		return "", stacktrace.Propagate(err, "error encoding jwt payload")
	}

	signature := joseBase64Encode(signatureBytes)

	return fmt.Sprintf("%s.%s", encodingToSign, signature), nil
}

// joseBase64Encode base64 encodes a byte slice then removes any padding
func joseBase64Encode(data []byte) string {
	return strings.TrimRight(base64.URLEncoding.EncodeToString(data), "=")
}

// accessEntry represents an access entry in a JWT.
type accessEntry struct {
	auth.Resource
	Actions []string `json:"actions"`
}
