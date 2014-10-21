# Contributing to the registry

## Are you having issues?

Please first try any of these support forums before opening an issue:

 * irc #docker on freenode (archives: [https://botbot.me/freenode/docker/])
 * https://forums.docker.com/
 * if your problem is with the "hub" (the website and other user-facing components), or about automated builds, then please direct your issues to https://support.docker.com

## So, you found a bug?

First check if your problem was already reported in the issue tracker.

If it's already there, please refrain from adding "same here" comments - these don't add any value and are only adding useless noise. **Said comments will quite often be deleted at sight**. On the other hand, if you have any technical, relevant information to add, by all means do!

Your issue is not there? Then please, create a ticket.

If possible the following guidelines should be followed:

 * try to come up with a minimal, simple to reproduce test-case
 * try to add a title that describe succinctly the issue
 * if you are running your own registry, please provide:
  * registry version
  * registry launch command used
  * registry configuration
  * registry logs
 * in all cases:
  * `docker version` and `docker info`
  * run your docker daemon in debug mode (-D), and provide docker daemon logs 

## You have a patch for a known bug, or a small correction?

Basic github workflow (fork, patch, make sure the tests pass, PR).

... and some simple rules to ensure quick merge:

 * clearly point to the issue(s) you want to fix
 * when possible, prefer multiple (smaller) PRs addressing individual issues over a big one trying to address multiple issues at once
 * if you need to amend your PR following comments, squash instead of adding more commits

## You want some shiny new feature to be added?

Fork the project.

Create a new proposal in the folder `open-design/specs`, named `DEP_MY_AWESOME_PROPOSAL.md`, using `open-design/specs/TEMPLATE.md` as a starting point.

Then immediately submit this new file as a pull-request, in order to get early feedback.

Eventually, you will have to update your proposal to accommodate the feedback you received.

Usually, it's not advisable to start working too much on the implementation itself before the proposal receives sufficient feedback, since it can significantly altered (or rejected).

Your implementation should then be submitted as a separate PR, that will be reviewed as well.
