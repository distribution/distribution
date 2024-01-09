group "default" {
  targets = ["image-local"]
}

group "validate" {
  targets = ["lint", "validate-git", "validate-vendor"]
}

target "lint" {
  dockerfile = "./dockerfiles/lint.Dockerfile"
  output = ["type=cacheonly"]
}

variable "COMMIT_RANGE" {
  default = ""
}
target "validate-git" {
  dockerfile = "./dockerfiles/git.Dockerfile"
  target = "validate"
  args = {
    COMMIT_RANGE = COMMIT_RANGE
    BUILDKIT_CONTEXT_KEEP_GIT_DIR = 1
  }
  output = ["type=cacheonly"]
}

target "validate-vendor" {
  dockerfile = "./dockerfiles/vendor.Dockerfile"
  target = "validate"
  output = ["type=cacheonly"]
}

target "update-vendor" {
  dockerfile = "./dockerfiles/vendor.Dockerfile"
  target = "update"
  output = ["."]
}

target "mod-outdated" {
  dockerfile = "./dockerfiles/vendor.Dockerfile"
  target = "outdated"
  no-cache-filter = ["outdated"]
  output = ["type=cacheonly"]
}

target "binary" {
  target = "binary"
  output = ["./bin"]
}

target "artifact" {
  target = "artifact"
  output = ["./bin"]
}

target "artifact-all" {
  inherits = ["artifact"]
  platforms = [
    "linux/amd64",
    "linux/arm/v6",
    "linux/arm/v7",
    "linux/arm64",
    "linux/ppc64le",
    "linux/s390x"
  ]
}

// Special target: https://github.com/docker/metadata-action#bake-definition
target "docker-metadata-action" {
  tags = ["registry:local"]
}

target "image" {
  inherits = ["docker-metadata-action"]
}

target "image-local" {
  inherits = ["image"]
  output = ["type=docker"]
}

target "image-all" {
  inherits = ["image"]
  platforms = [
    "linux/amd64",
    "linux/arm/v6",
    "linux/arm/v7",
    "linux/arm64",
    "linux/ppc64le",
    "linux/s390x"
  ]
}

target "_common_docs" {
  dockerfile = "./dockerfiles/docs.Dockerfile"
}

target "docs-export" {
  inherits = ["_common_docs"]
  target = "out"
  output = ["type=local,dest=build/docs"]
}

target "docs-image" {
  inherits = ["_common_docs"]
  target = "server"
  output = ["type=docker"]
  tags = ["registry-docs:local"]
}

target "docs-test" {
  inherits = ["_common_docs"]
  target = "test"
  output = ["type=cacheonly"]
}

target "authors" {
  dockerfile = "./dockerfiles/authors.Dockerfile"
  target = "update"
  output = ["."]
}

target "validate-authors" {
  dockerfile = "./dockerfiles/authors.Dockerfile"
  target = "validate"
  output = ["type=cacheonly"]
}
