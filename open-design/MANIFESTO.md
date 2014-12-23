# The "Distribution" project

## What is this

This is a part of the Docker project, or "primitive" that handles the "distribution" of images.

### Punchline

Pack. Sign. Ship. Store. Deliver. Verify.

### Technical scope

Distribution has tight relations with:

 * libtrust, providing cryptographical primitives to handle image signing and verification
 * image format, as transferred over the wire
 * docker-registry, the server side component that allows storage and retrieval of packed images
 * authentication and key management APIs, that are used to verify images and access storage services
 * PKI infrastructure
 * docker "pull/push client" code gluing all this together - network communication code, tarsum, etc
