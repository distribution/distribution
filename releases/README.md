## Registry Release Checklist

10. Compile release notes detailing features added since the last release.

  Add release template file to `releases/` directory. The template is defined
by containerd's release tool. Name the file using the version, for rc add
an `-rc` suffix.
See https://github.com/containerd/containerd/tree/master/cmd/containerd-release

20. Update the `.mailmap` files.

30. Update the version file: `https://github.com/docker/distribution/blob/master/version/version.go`

40. Create a signed tag.

  Choose a tag for the next release, distribution uses semantic versioning
and expects tags to be formatted as `vx.y.z[-rc.n]`. Run the release tool using
the release template file and tag to generate the release notes for the tag
and Github release. To create the tag, you will need PGP installed and a PGP
key which has been added to your Github account. The comment for the tag will
be the generate release notes, always compare with previous tags to ensure
the output is expected and consistent.
Run `git tag --cleanup=whitespace -s vx.y.z[-rc.n] -F release-notes` to create
tag and `git -v vx.y.z[-rc.n]` to verify tag, check comment and correct commit
hash.

50. Push the signed tag

60. Create a new [release](https://github.com/docker/distribution/releases).
In the case of a release candidate, tick the `pre-release` checkbox. Use
the generate release notes from the release tool

70. Update the registry binary in the [distribution library image repo](https://github.com/docker/distribution-library-image) by running the update script and  opening a pull request.

80. Update the official image.  Add the new version in the [official images repo](https://github.com/docker-library/official-images) by appending a new version to the `registry/registry` file with the git hash pointed to by the signed tag.  Update the major version to point to the latest version and the minor version to point to new patch release if necessary.
e.g. to release `2.3.1`

   `2.3.1 (new)`

   `2.3.0 -> 2.3.0` can be removed

   `2 -> 2.3.1`

   `2.3 -> 2.3.1`

90. Build a new distribution/registry image on [Docker hub](https://hub.docker.com/u/distribution/dashboard) by adding a new automated build with the new tag and re-building the images.
