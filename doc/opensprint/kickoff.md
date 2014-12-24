Distribution
=========================

## Project intentions

**Problem statement and requirements**

* What is the exact scope of the problem?


Design a professional grade and extensible content distribution system, that allows docker users to:

... by default enjoy:

	* an efficient, secured and reliable way to store, manage, package and exchange content

... optionally:

	* can hack/roll their own on top of healthy open-source components

... with the liberty to:

	* implement their own home made solution through good specs, and solid extensions mechanism


* Who will the result be useful to?

	* users
	* ISV (who distribute images or develop image distribution solutions)
	* docker

* What are the use cases (distinguish dev & ops population where applicable)?

	* Everyone (... uses docker push/pull).

* Why does it matter that we build this now?

	* Shortcomings of the existing codebase are the #1 pain point (by large) for users, partners and ISV, hence the most urgent thing to address (?)
	* That situation is getting worse everyday and killer competitors are going/have emerged. 

* Who are the competitors?

	* existing artifact storage solutions (eg: artifactory).
	* emerging products that aim at handling pull/push in place of docker.
	* ISV that are looking for alternatives to workaround this situation

**Current state: what do we have today?**

Problems of the existing system:

1. not reliable
	* registry goes down whenever the hub goes down
	* failing push result in broken repositories
	* concurrent push is not handled
	* python boto and gevent have a terrible history
	* organically grown, under-designed features are in a bad shape (search)
2. inconsistent
	* discrepancies between duplicated API (and *duplicated APIs*)
	* unused features
	* missing essential features (proper SSL support)
3. not reusable
	* tightly entangled with hub component makes it very difficult to use outside of docker
 	* proper access-control is almost impossible to do right
 	* not easily extensible
4. not efficient
	* no parallel operations (by design)
	* sluggish client-side processing / bad pipeline design
	* poor reusability of content (random ids)
	* scalability issues (tags)
	* too many useless requests (protocol)
	* too much local space consumed (local garbage collection: broken + not efficient)
	* no squashing
5. not resilient to errors
	* no resume
	* error handling is obscure or inexistent
6. security
	* content is not verified
	* current tarsum is broken 
	* random ids are a headache
7. confusing
	* registry vs. registry.hub?
	* layer vs. image?
8. broken features
	* mirroring is not done correctly (too complex, bug-laden, caching is hard)
9. poor integration with the rest of the project
	* technology discrepancy (python vs. go)
	* poor testability
	* poor separation (API in the engine is not defined enough)
10. missing features / prevents future
	* trust / image signing
	* naming / transport separation
	* discovery / layer federation
	* architecture + os support (eg: arm/windows)
	* quotas
	* alternative distribution methods (transport plugins)

**Future state: where do we want to get?**

* Deliverable
	* new JSON/HTTP protocol specification
	* new image format specification
	* (new image store in the engine)
	* new transport API between the engine and the distribution client code / new library
	* new registry in go
	* new authentication service on top of the trust graph in go

* What are the interactions with other components of the project?
	* critical interactions with docker push/pull mechanism
	* critical interactions with the way docker stores images locally

* In what way will the result be customizable?
	* transport plugins allowing for radically different transport methods (bittorent, direct S3 access, etc)
	* extensibility design for the registry allowing for complex integrations with other systems
	* backend storage drivers API


## Kick-off output

**What is the expected output of the kick-off session?**

* draft specifications
* separate binary tool for demo purpose
* a mergeable PR that fixes 90% of the listed issues


* agree on a vision that allows solving all that are deemed worthy
* propose a long term battle plan with clear milestones that encompass all these
* define a first milestone that is compatible with the future and does already deliver some of the solutions
* deliver the specifications for image manifest format and transport API
* deliver a working implementation that can be used as a drop-in replacement for the existing v1 with an equivalent feature-set

**How is the output going to be demoed?**

docker pull
docker push

**Once demoed, what will be the path to shipping?**

A minimal PR that include the first subset of features to make docker work well with the new server side components.

## Pressing matters

 * need a codename (ship, distribute)
 * new repository
 * new domains

 * architecture / OS
 * persistent ids
 * registries discovery
 * naming (quay.io/foo/bar)
 * mirroring



## Assorted issues

 * some devops want a docker engine that cannot do push/pull

