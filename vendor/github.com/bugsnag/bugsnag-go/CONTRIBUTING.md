Contributing
============

-   [Fork](https://help.github.com/articles/fork-a-repo) the [notifier on github](https://github.com/bugsnag/bugsnag-go)
-   Build and test your changes
-   Commit and push until you are happy with your contribution
-   [Make a pull request](https://help.github.com/articles/using-pull-requests)
-   Thanks!


Installing the go development environment
-------------------------------------

1.  Install homebrew

    ```
    ruby -e "$(curl -fsSL https://raw.github.com/Homebrew/homebrew/go/install)"
    ```

1. Install go

    ```
    brew install go --cross-compile-all
    ```

1. Configure `$GOPATH` in `~/.bashrc`

    ```
    export GOPATH="$HOME/go"
    export PATH=$PATH:$GOPATH/bin
    ```

Downloading the code
--------------------

You can download the code and its dependencies using

```
go get -t github.com/bugsnag/bugsnag-go
```

It will be put into "$GOPATH/src/github.com/bugsnag/bugsnag-go"

Then install depend


Running Tests
-------------

You can run the tests with

```shell
go test
```

Making PRs
----------

All PRs should target the `next` branch as their base. This means that we can land them and stage them for a release without making multiple changes to `master` (which would cause multiple releases due to `go get`'s behaviour).

The exception to this rule is for an urgent bug fix when `next` is already ahead of `master`. See [hotfixes](#hotfixes) for what to do then.

Releasing a New Version
-----------------------

If you are a project maintainer, you can build and release a new version of
`bugsnag-go` as follows:

#### Planned releases

**Prerequisite**: All code changes should already have been reviewed and PR'd into the `next` branch before making a release.

1. Decide on a version number and date for this release
1. Add an entry (or update the `TBD` entry if it exists) for this release in `CHANGELOG.md` so that it includes the version number, release date and granular description of what changed
1. Update the README if necessary
1. Update the version number in `bugsnag.go` and verify that tests pass.
1. Commit these changes `git commit -am "Preparing release"`
1. Create a PR from `next` -> `master` titled `Release vX.X.X`, adding a description to help the reviewer understand the scope of the release
1. Await PR approval and CI pass
1. Merge to master on GitHub, using the UI to set the merge commit message to be `vX.X.X`
1. Create a release from current `master` on GitHub called `vX.X.X`. Copy and paste the markdown from this release's notes in `CHANGELOG.md` (this will create a git tag for you).
1. Ensure setup guides for Go (and its frameworks) on docs.bugsnag.com are correct and up to date.
1. Merge `master` into `next` (since we just did a merge commit the other way, this will be a fastforward update) and push it so that it is ready for future PRs.


#### Hotfixes

If a `next` branch already exists and is ahead of `master` but there is a bug fix which needs to go out urgently, check out the latest `master` and create a new hotfix branch `git checkout -b hotfix`. You can then proceed to follow the above steps, substituting `next` for `hotfix`.

Once released, ensure `master` is merged into `next` so that the changes made on `hotfix` are included.
