Storage Adapters
================

The go-ipld-prime storage APIs were introduced in the v0.14.x ranges of go-ipld-prime,
which happened in fall 2021.

There are many other pieces of code in the IPLD (and even more so, the IPFS) ecosystem
which predate this, and have interfaces that are very _similar_, but not quite exactly the same.

In order to keep using that code, we've built a series of adapters.

You can see these in packages beneath this one:

- `go-ipld-prime/storage/bsadapter` is an adapter to `github.com/ipfs/go-ipfs-blockstore`.
- `go-ipld-prime/storage/dsadapter` is an adapter to `github.com/ipfs/go-datastore`.
- `go-ipld-prime/storage/bsrvadapter` is an adapter to `github.com/ipfs/go-blockservice`.

Note that there are also other packages which implement the go-ipld-prime storage APIs,
but are not considered "adapters" -- these just implement the storage APIs directly:

- `go-ipld-prime/storage/memstore` is a simple in-memory storage system.
- `go-ipld-prime/storage/fsstore` is a simple filesystem-backed storage system
  (comparable to, and compatible with [flatfs](https://pkg.go.dev/github.com/ipfs/go-ds-flatfs),
  if you're familiar with that -- but higher efficiency).

Finally, note that there are some shared benchmarks across all this:

- check out `go-ipld-prime/storage/benchmarks`!


Why structured like this?
-------------------------

### Why is there adapter code at all?

The `go-ipld-prime/storage` interfaces are a newer generation.

A new generation of APIs was desirable because it unifies the old APIs,
and also because we were able to improves and update several things in the process.
(You can see some of the list of improvements in https://github.com/ipld/go-ipld-prime/pull/265,
where these APIs were first introduced.)
The new generation of APIs avoids several types present in the old APIs which forced otherwise-avoidable allocations.
(See notes later in this document about "which adapter should I use" for more on that.)
Finally, the new generation of APIs is carefully designed to support minimal implementations,
by carefully avoiding use of non-standard-library types in key API definitions,
and by keeping most advanced features behind a standardized convention of feature detection.

Because the newer generation of APIs are not exactly the same as the multiple older APIs we're unifying and updating,
some amount of adapter code is necessary.

(Fortunately, it's not much!  But it's not "none", either.)

### Why have this code in a shared place?

The glue code to connect `go-datastore` and the other older APIs
to the new `go-ipld-prime/storage` APIs is fairly minimal...
but there's also no reason for anyone to write it twice,
so we want to put it somewhere easy to share.

### Why do the adapters have their own go modules?

A separate module is used because it's important that go-ipld-prime can be used
without forming a dependency on `go-datastore` (or the other relevant modules, per adapter).

We want this so that there's a reasonable deprecation pathway -- it must be
possible to write new code that doesn't take on transitive dependencies to old code.

(As a bonus, looking at the module dependency graphs makes an interestingly
clear statement about why minimal APIs that don't force transitive dependencies are a good idea!)

### Why is this code all together in this repo?

We put these separate modules in the same git repo as `go-ipld-prime`... because we can.

Technically, neither the storage adapter modules nor the `go-ipld-prime` module depend on each other --
they just have interfaces that are aligned with each other -- so it's very easy to
hold them as separate go modules in the same repo, even though that can otherwise sometimes be tricky.

You may want to make a point of pulling updated versions of the storage adapters that you use
when pulling updates to go-ipld-prime, though.

### Could we put these adapters upstream into the other relevant repos?

Certainly!

We started with them here because it seemed developmentally lower-friction.
That may change; these APIs could move.
This code is just interface satisfaction, so even having multiple copies of it is utterly harmless.


Which of `dsadapter` vs `bsadapter` vs `bsrvadapter` should I use?
------------------------------------------------------------------

None of them, ideally.
A direct implementation of the storage APIs will almost certainly be able to perform better than any of these adapters.
(Check out the `fsstore` package, for example.)

Failing that: use the adapter matching whatever you've got on hand in your code.

There is no correct choice.

`dsadapter` suffers avoidable excessive allocs in processing its key type,
due to choices in the interior of `github.com/ipfs/go-datastore`.
It is also unable to support streaming operation, should you desire it.

`bsadapter` and `bsrvadapter` both also suffer overhead due to their key type,
because they require a transformation back from the plain binary strings used in the storage API to the concrete go-cid type,
which spends some avoidable CPU time (and also, at present, causes avoidable allocs because of some interesting absenses in `go-cid`).
Additionally, they suffer avoidable allocs because they wrap the raw binary data in a "block" type,
which is an interface, and thus heap-escapes; and we need none of that in the storage APIs, and just return the raw data.
They are also unable to support streaming operation, should you desire it.

It's best to choose the shortest path and use the adapter to whatever layer you need to get to --
for example, if you really want to use a `go-datastore` implementation,
*don't* use `bsadapter` and have it wrap a `go-blockstore` that wraps a `go-datastore` if you can help it:
instead, use `dsadapter` and wrap the `go-datastore` without any extra layers of indirection.
You should prefer this because most of the notes above about avoidable allocs are true when
the legacy interfaces are communicating with each other, as well...
so the less you use the internal layering of the legacy interfaces, the better off you'll be.

Using a direct implementation of the storage APIs will suffer none of these overheads,
and so will always be your best bet if possible.

If you have to use one of these adapters, hopefully the performance overheads fall within an acceptable margin.
If not: we'll be overjoyed to accept help porting things.
