# Releasing

This document describes the release process, tools, conventions, and anything else related to maintaining releases.

## Tools and Conventions

To automate the process of creating releases while adhering to Semantic Versioning (SemVer), this project uses
[Semantic Release](https://github.com/semantic-release/semantic-release).
[Semantic Release](https://github.com/semantic-release/semantic-release) requires standardized commit messages, to fullfil this requirement this project uses [Conventional Commit](https://www.conventionalcommits.org/en/v1.0.0/#specification).
[Semantic Release](https://github.com/semantic-release/semantic-release) uses the commit messages to determine what the
next version of a release should be and if a new version should be released. If no commits are found that match the
convention, no release will be created. Otherwise, a Git tag will be created and a GitHub release is generated with the
changelog according to the parsed commits.

### Commit Convention Enforcement

While the project strives to use [Conventional Commit](https://www.conventionalcommits.org/en/v1.0.0/#specification)
for all commits made to the repo, the maintainers acknowledge this puts a high burden on contributors.
Our CI checks if all commits follow [Conventional Commit](https://www.conventionalcommits.org/en/v1.0.0/#specification), however,
this CI mainly there to help and remind people about the commit convention for the time being. To ensure all PRs are properly
taken into account for new releases, PR titles **must** also follow the convention and PRs must be merged with the PR title
as the merge message. This allows maintainers to easily edit and correct the PR titles as necessary so they conform to
[Conventional Commit](https://www.conventionalcommits.org/en/v1.0.0/#specification) without burdening contributors.

**The settings for the project's GitHub repo need to default PR merges to the title of the PR for the merge message.**

## Branches and Workflow

[Semantic Release](https://github.com/semantic-release/semantic-release) requires identifiable Git tags and branch naming to work properly.
While it takes care of creating the Git tags, maintainers are responsible for creating Git branches following the correct naming convention.
A good overview of how the workflow works and can be configured can be found in the Semantic Release [docs](https://semantic-release.gitbook.io/semantic-release/usage/workflow-configuration).
We recommend all maintainers to read through these docs.
Maintenance branches must be named following the `^([0-9])?(\.([0-9]|x))?\.x` regex. For example `2.x` or `3.0.x`, where the
`2.x` branch can receive commits with both fixes and features releasing new minor or patch versions, while the `3.0.x` branch
only accepts commits with fixes and new path versions are released from it. For `alpha`, `beta`, and `rc` releases those are
created from the branches with the same name.

### Workflow Overview

It is recommended to read the Semantic Release [docs](https://semantic-release.gitbook.io/semantic-release/recipes/release-workflow/maintenance-releases)
on how maintenance releases work before continuing on reading this section.

The following section will use the upcoming v3 release as a practical example.
The default release branch is `main`, meaning that the current release originates from the `main` branch.
To cut the first `alpha` release for v3 the branch `alpha` must be created off of the `main` branch and the release automation
should be run on the `alpha` branch (the `v3.0.0-alpha.1` tag is then created).
See [Manually Running a Single Release](#manually-running-a-single-release) for how to run the automation on only the `alpha`
branch. If during the alpha stage new commits are added and a new release should be created, the `main` branch should be merged
into the `alpha` branch and the release automation should be run again. The same goes for `beta` and `rc` releases. Once we are
ready to release v3 it's as simple as running the release automation on the `main` branch. This will create the `v3.0.0` tag on
the `main` branch.

After the initial release of v3 on the `main` branch, new features and fixes are continued to be pushed to the `main` branch.

**Note: No maintenance release branches such as `v3.x` or `v3.0.x` should be created at this point.**

Maintenance release branches are created from Git release tags once they are required to backport a fix to a previous version.
For example, after the `v3.0.0` release new commits are pushed to the `main` branch that have resulted in the versions `v3.0.1`,
`v3.1.0`, `v3.1.1` and `v3.2.0` being released. Next, a CVE is found and the fix is committed to the `main` branch and `v3.2.1`
is released. We want to backport this fix to users of `v3.0.x` and `v3.1.x`. To do this the branch `v3.0.x` is created from the
`v3.0.1` Git tag and the branch `v3.1.x` is created from the `v3.1.1` Git tag. Then the commit that addresses the CVE is
cherry-picked onto those branches and the release automation is run on those branches, which will release version `v3.0.2` and
`v3.1.2`. Although strictly speaking this backporting shouldn't be needed since all users should be able to update to `v3.2.1`
safely, as it only contains new features and no breaking changes.

Having the maintenance branches be created as they are needed allows for a much cleaner branching structure and doesn't require
complex automation around getting commits from the `main` branch into the current release branch.

### Major Versions

Once breaking changes are needed and we want to start working on the next major version the branch `next` should be created.
Breaking changes can be pushed to that branch and the release automation can run there. Once we are ready to release the
next major version the branch `next` is merged into the `main` branch to release v4. Please see [this](https://semantic-release.gitbook.io/semantic-release/usage/workflow-configuration#pushing-to-a-release-branch) doc for more information.

### Ensuring Valid Branch Commits

To ensure only the correct type of commits (fixes, features, breaking changes) are pushed to release and maintenance branches
CI automation is setup to validate a release branch after a PR is merged. This is done by checking out the base ref of a PR,
merging the PR with the PR title as the merge message (which should follow the convention), and then running semantic release
in `dry-run` mode. For example, this way a PR with the title `feat:` or commits with that message can't be merged into the
`v3.0.x` branch. If the PR is updated to change its title this CI will run again. If for whatever reason a bad commit is added
to a release or maintenance branch, the release automation will fail and will show the offending commit and how it can be solved.

The above mentioned CI is in the form of a GitHub action and can be found [here](.github/workflows/release-validate.yml).

## Release Automation

This section goes into detail how the release automation works and how it can be used to trigger a new release.

The GitHub Action that creates a release for a particular branch can be found [here](.github/workflows/release.yml).
A separate GitHub Action is used to run the release action mentioned above on all release and maintenance branches.
This is necessary due to how the GitHub context works along with the ability to schedule releases automatically in the future.
The action that will run the release automation on all release and maintenance branches can be found [here](.github/workflows/scheduled-release.yml).
At the time of writing the schedule of the scheduled-release action is disabled while we are still getting familiar with the release process.

### Scheduled Releases

To ensure continued project momentum and a predictable release cadence for users and contributors alike, the intention is
for the release automation to be run on a schedule. To enable this, the cron schedule in the action found
[here](.github/workflows/scheduled-release.yml) needs to be uncommented.

### Manually Running a Single Release

To manually run a release for a particular release or maintenance branch:

Go to the `Actions` tab of the repository:

![Actions](./docs/images/actions_tab.png)

Select the `release` action in the sidebar:

![Release Sidebar](./docs/images/release_sidebar.png)

Select the branch you want to run a release for and run the action:

![Run Single Release](./docs/images/run_single_release.png)

### Manually Running All Releases

To manually run a release on all release and maintenance branches:

Go to the `Actions` tab of the repository:

![Actions](./docs/images/actions_tab.png)

Select the `scheduled-release` action in the sidebar:

![Scheduled Release Sidebar](./docs/images/scheduled_release_sidebar.png)

Leave the `main` branch selected and run the action:

![Run All Releases](./docs/images/run_all_releases.png)
