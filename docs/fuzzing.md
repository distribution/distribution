# Fuzzing

The six Go fuzz targets are exposed through `Taskfile.fuzz.yml`, using the same
`fuzz:list`, `fuzz:run`, `fuzz:coverage` and `fuzz:replay` interface as Deckhouse,
Stronghold, storage-volume-data-manager and Prom++.

## CI and image

`werf.yaml` prepares `distribution-artifact` from this checkout and invokes
`include "fuzz image"` from `.werf/defines/fuzz.tmpl`, matching the structure
used by neighbouring repositories. The template imports the prepared sources,
installs fuzz tooling, restores the S3 corpus and replays the targets.

GitLab CI builds `distribution-fuzz` with werf. The image contains the checkout,
vendored dependencies, Go, Task, Python, jq and AWS CLI. Its working directory is
`/src`, with `Taskfile.fuzz.yml` copied to `/src/Taskfile.yml` and the
`io.deckhouse.fuzz.engine=go` label used by the fuzzing platform.

- Merge requests build locally and replay the default branch's corpus without
  pushing an image or uploading a build report. The default branch here is
  `deckhouse`; the common CI template's hardcoded `main` is not used for replay.
- Default-branch pipelines build and publish using the shared
  `Build_Fuzz_Images.gitlab-ci.yml` template. It uploads
  `images_fuzz_tags_werf.json`, including GitLab source metadata, to
  `s3://anomaloys-materials/build-reports/<project>/<branch-slug>/`.
- During the build, the corpus is restored from
  `s3://anomaloys-materials/<project>/<branch-slug>/` and each target is replayed.
  Any discovery, download or replay failure fails the build. Restored inputs are
  removed after replay; the platform restores the current corpus at runtime.
- Continuous fuzzing is run by the platform against the published image. Image
  builds only replay seeds and the saved corpus.

The GitLab project needs a Linux amd64 runner tagged `deckhouse`, with Docker,
`trdl`, Bash, curl, unzip and jq, plus access to the shared CI project. Set
`DEV_MODULES_REGISTRY`, `DEV_MODULES_REGISTRY_LOGIN` and
`DEV_MODULES_REGISTRY_PASSWORD`. The default image repository is
`<DEV_MODULES_REGISTRY>/sys/deckhouse-oss/modules/<CI_PROJECT_NAME>`; override
`MODULES_MODULE_SOURCE`/`MODULES_MODULE_NAME` if this fork uses another namespace.

The shared job obtains `FUZZ_S3_ENDPOINT`, `FUZZ_S3_ACCESS_KEY` and
`FUZZ_S3_SECRET_KEY` from Vault using the GitLab ID token. `VAULT_AUTH_ROLE`
defaults to `CI_PROJECT_NAME`; configure a role authorized for this GitLab
project, or override the variable with an existing authorized role. Credentials
enter the build through werf secrets and are not stored in the image.

## Local commands

Use Go 1.25 or later and Task 3.50 or later:

```sh
task --taskfile Taskfile.fuzz.yml fuzz:list
FUZZ_PKG=./registry/proxy FUZZ_TARGET=FuzzProxyCachePoisoning \
  task --taskfile Taskfile.fuzz.yml fuzz:replay
FUZZ_PKG=./registry/proxy FUZZ_TARGET=FuzzProxyCachePoisoning FUZZ_TIME=60s \
  task --taskfile Taskfile.fuzz.yml fuzz:run
FUZZ_PKG=./registry/proxy FUZZ_TARGET=FuzzProxyCachePoisoning \
  FUZZ_COVERAGE_FILE=/tmp/distribution-fuzz-coverage.out \
  task --taskfile Taskfile.fuzz.yml fuzz:coverage
```

`FUZZ_WORKERS` defaults to 4 because the HTTP targets can exhaust ephemeral
ports with excessive parallelism. Without `FUZZ_TIME`, `fuzz:run` continues
until stopped by the platform. The commands use `-mod=vendor` and can run
without downloading application dependencies.

## Existing failures

The current seed corpus reproduces two pre-existing defects:

- `FuzzProxyHeadersClientCert`: an untrusted, expired or unsuitable client leaf
  can inherit trust from another certificate supplied in the chain.
- `FuzzManifestPut/seed#4`: a manifest with an absent schema version produces
  HTTP 500 instead of a client error.

These failures block image publication. CI does not skip either target or set
`FUZZ_ALLOW_KNOWN_5XX`. For a local investigation of other manifest inputs only,
the existing `FUZZ_ALLOW_KNOWN_5XX=1` switch can bypass the second finding.
