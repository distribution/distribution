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

Usually, it's not advisable to start working too much on the implementation itself before the proposal receives sufficient feedback, since it can be significantly altered (or rejected).

Your implementation should then be submitted as a separate PR, that will be reviewed as well.

## Issue and PR labels

To keep track of the state of issues and PRs, we've adopted a set of simple labels. The following are currently in use:

<dl>
	<dt><a href="https://github.com/docker/distribution/issues?q=is%3Aopen+-label%3AReady+-label%3A%22In+Progress%22+-label%3A%22Blocked%22">Backlog</a></dt>
	<dd>Issues marked with this label are considered not yet ready for implementation. Either they are untriaged or require futher detail to proceed.</dd>

	<dt><a href="https://github.com/docker/distribution/labels/Blocked">Blocked</a></dt>
	<dd>If an issue requires further clarification or is blocked on an unresolved dependency, this label should be used.</dd>

	<dt><a href="https://github.com/docker/distribution/labels/Sprint">Sprint</a></dt>
	<dd>Issues marked with this label are being worked in the current sprint. All required information should be available and design details have been worked out.</dd>

	<dt><a href="https://github.com/docker/distribution/labels/In%20Progress">In Progress</a></dt>
	<dd>The issue or PR is being actively worked on by the assignee.</dd>

	<dt><a href="https://github.com/docker/distribution/issues?q=is%3Aclosed">Done</a></dt>
	<dd>Issues marked with this label are complete. This can be considered a psuedo-label, in that if it is closed, it is considered "Done".</dd>
</dl>

These integrate with waffle.io to show the current status of the project. The project board is available at the following url:

https://waffle.io/docker/distribution

If an issue or PR is not labeled correctly or you believe it is not in the right state, please contact a maintainer to fix the problem.

## Milestones

Issues and PRs should be assigned to relevant milestones. If an issue or PR is assigned a milestone, it should be available by that date. Depending on level of effort, items may be shuffled in or out of milestones. Issues or PRs that don't have a milestone are considered unscheduled. Typically, "In Progress" issues should have a milestone.

## PR Titles

PR titles should be lowercased, except for proper noun references (such a
method name or type).

PR titles should be prefixed with affected directories, comma separated. For
example, if a specification is modified, the prefix would be "doc/spec". If
the modifications are only in the root, do not include it. If multiple
directories are modified, include each, separated by a comma and space.

Here are some examples:

- doc/spec: move API specification into correct position
- context, registry, auth, auth/token, cmd/registry: context aware logging
