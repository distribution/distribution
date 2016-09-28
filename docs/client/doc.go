// package client is a helper package for the DTR<>Registry API endpoints. For
// example, deleting a repository within DTR is complex compared to registry as we
// need to delete all tags from blob and metadata store, then delete the repo from
// the DTR DB.
//
// This is compared to plain registry when nuking the entire repository directory
// would suffice.
package client
