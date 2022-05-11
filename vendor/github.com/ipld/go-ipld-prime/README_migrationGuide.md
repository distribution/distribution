A short guide to migrating to go-ipld-prime from other repos
============================================================

Here are some quick notes on APIs that you might've been using
if you worked with IPLD in golang before go-ipld-prime,
and a pointer to what you should check out for an equivalent
in order to upgrade your code to use go-ipld-prime.

(Let us know if there are more pointers we should add to this list to ease your journey,
or someone else's future journey!)

- Were you using [ipfs/go-datastore](https://pkg.go.dev/github.com/ipfs/go-datastore) APIs?
	- You can wrap those in `storage/dsadapter` and keep using them.
	  You can also plug that into `linking.LinkSystem` to get higher level IPLD operations.
	- Or if you were only using datastore because of some specific implementation of it, like, say `flatfs`?
	  Then check out the possibility of moving all the way directly to new implementations like `storage/fsstore`.
- Were you using [ipfs/go-ipfs-blockstore](https://pkg.go.dev/github.com/ipfs/go-ipfs-blockstore) APIs?
	- You can wrap those in `storage/bsadapter` and keep using them.
	  You can also plug that into `linking.LinkSystem` to get higher level IPLD operations.
	  (This is almost exactly the same; we've just simplified in the interface, made it easier to implement, and cleaned up inconsistencies with the other interfaces in this migration guide which were already very very similar.)
- Were you using [ipfs/go-blockservice](https://pkg.go.dev/github.com/ipfs/go-blockservice) APIs?
	- You can wrap those in `storage/bsrvadapter` and keep using them.
	  You can also plug that into `linking.LinkSystem` to get higher level IPLD operations.
		- Be judicious about whether you actually want to do this.
		  Plugging in the potential to experience unknown network latencies into code that's expecting predictable local lookup speeds
		  may have undesirable performance outcomes.  (But if you want to do it, go ahead...)
- Were you using [ipfs/go-ipld-format.DAGService](https://pkg.go.dev/github.com/ipfs/go-ipld-format#DAGService)?
	- If you're using the `Add` and `Get` methods: those are now on `linking.LinkSystem`, as `Store` and `Load`.
	- If you're using the `*Many` methods: those don't have direct replacements;
	  we don't use APIs that force that style of future-aggregation anymore, because it's bad for pipelining.
	  Just use `Store` and `Load`.
- Were you using [ipfs/go-ipld-format.Node](https://pkg.go.dev/github.com/ipfs/go-ipld-format#Node)?
	- That's actually a semantic doozy -- we use the word "node" _much_ differently now.
	  See https://ipld.io/glossary/#node for an introduction.
	  (Long story short: the old "node" was often more like a whole block, and the new "node" is more like an AST node: there's lots of "node" in a "block" now.)
	- There's an `datamodel.Node` interface -- it's probably what you want.
	  (You just might have to update how you think about it -- see above bullet point about semantics shift.)
	  It's also aliased as the `ipld.Node` interface here right in the root package, because it's such an important and central interface that you'll be using it all the time.
	- Are you specifically looking for how to get a list of links out of a block?
	  That's a `LinkSystem.Load` followed by applying `traversal.SelectLinks` on the node it gives you: now you've got the list of links from that node (and its children, recursively, as long as they came from the same block of raw data), tada!
- Are you looking for the equivalent of [ipfs/go-ipld-format.Link](https://pkg.go.dev/github.com/ipfs/go-ipld-format#Link)?
	- That's actually a feature specific to dag-pb, and arguably even specific to unixfsv1.
	  There is no equivalent in go-ipld-prime as a result.
	  But you'll find it in other libraries that are modern and go-ipld-prime based: see below.
- Were you using some feature specific to dag-pb?
	- There's an updated dag-pb codec found in https://github.com/ipld/go-codec-dagpb -- it should have what you need.
	- Does it turn out you actually meant you're using some feature specific to unixfsv1?
	  (Yeah -- that comes up a lot.  It's one of the reasons we're deprecating so many of the old libraries -- they make it _really_ easy to confuse this.)
	  Then hang on for the next bullet point, which is for you! :)
- Were you using some features of unixfsv1?
	- Check out the https://github.com/ipfs/go-unixfsnode library -- it should have what you need.
		- Pathing, like what used to be found in ipfs/go-path?  That's here now.
		- Sharding?  That's here now too.
		- You probably don't need incentives if you're here already, but can we quickly mention that... you can use Selectors over these?  Neat!

If you're encountering more questions about how to migrate some code,
please jump in to the ipld chat via either [matrix](https://matrix.to/#/#ipld:ipfs.io) or discord (or any other bridge)
and ask!
We want to grow this list of pointers to be as encompassing and helpful as possible.
