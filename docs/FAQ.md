# Registry past-and-present cheat sheet

> Everything You Always Wanted to Know about the Registry

## What's a Registry?

It's a server-side storage and delivery software that let you publish and exchange docker images.

Whenever you do a `docker pull` or `docker push`, your engine communicates with a registry.

## So, does the Docker Hub contain a registry?

Yes.

That "Hub registry" is operated by Docker.

It is free to use for public content, and offers paid-for private storage as well.

Support for the hub is handled directly by Docker, over here: https://support.docker.com

## What if I don't want to rely on the Docker Hub?

Then you should run and operate your own registry. 

While this solution will require more work from you (like running and operating any kind of service does), it will give you more control over your images, and more configurability.

## Are there different registries implementations available?

Yes, there exist at least two different.

First, the "old, legacy registry":

 * implemented in python
 * source code is here: https://github.com/docker/docker-registry
 * supports the v1 protocol
 * is currently in maintenance mode: only critical security fixes are being worked on

Second, the "new registry":

 * implemented in go
 * source code is here: https://github.com/docker/distribution
 * implements a new, "v2" protocol
 * is were most of the development is happening, and the right place for new and upcoming ideas 


## Which one should I use then?


### Rule of thumb

... and strong recommandation: please use the latest Docker version available (>=1.6), and the new, golang registry.

The image for the new registry is here:

 * https://registry.hub.docker.com/u/library/registry2/
 * basic introduction about it is here: https://docs.docker.com/registry/
 * advanced documentation on the github repository here: https://github.com/docker/distribution

### Yeah,but: "I have been running the old registry - I want to upgrade and don't look back, but I want to keep my existing images"

The recommandation to do that is:

 * your old registry is running at address "my.internal.registry"
 * setup your new registry at a new address "temporary.registry"
 * using docker 1.6, pull images from your legacy registry, and push them to your new registry
 * ensure all your users are upgraded to docker 1.6
 * stop your legacy registry
 * start your new registry on address "my.internal.registry"

### Or: "I have been running the old registry - I want to start using the new one, but also keep the old one simultaneously"

The only reason why you would want to do that is if you want to keep your registry service running for older (< 1.6) docker engine versions, but also be able to start using the new registry for newer engines (>= 1.6).

To achieve that, you will have to run both a legacy registry, and a new registry, both behind a nginx proxy doing the routing.

TODO: details about that

### Yelling: "I DONT WANT TO CHANGE! WHY DO I HAVE TO CHANGE? YOU MORONS!"

Good news for you: "YOU DONT HAVE TO CHANGE ANYTHING".

You can keep running your old registry with newer docker engine versions. The docker engine is retro-compatible and will happily use whatever registry you are running.

