// The storage package contains interfaces for storage systems, and functions for using them.
//
// These are very low-level storage primitives.
// The interfaces here deal only with raw keys and raw binary blob values.
//
// In IPLD, you can often avoid dealing with storage directly yourself,
// and instead use linking.LinkSystem to handle serialization, hashing, and storage all at once.
// (You'll hand some values that match interfaces from this package to LinkSystem when configuring it.)
// It's probably best to work at that level and above as much as possible.
// If you do need to interact with storage more directly, the read on.
//
// The most basic APIs are ReadableStorage and WritableStorage.
// When writing code that works with storage systems, these two interfaces should be seen in almost all situations:
// user code is recommended to think in terms of these types;
// functions provided by this package will accept parameters of these types and work on them;
// implementations are expected to provide these types first;
// and any new library code is recommended to keep with the theme: use these interfaces preferentially.
//
// Users should decide which actions they want to take using a storage system,
// find the appropriate function in this package (n.b., package function -- not a method on an interface!
// You will likely find one of each, with the same name: pick the package function!),
// and use that function, providing it the storage system (e.g. either ReadableStorage, WritableStorage, or sometimes just Storage)
// as a parameter.
// That function will then use feature-detection (checking for matches to the other,
// more advanced and more specific interfaces in this package) and choose the best way
// to satisfy the request; or, if it can't feature-detect any relevant features,
// the function will fall back to synthesizing the requested behavior out of the most basic API.
// Using the package functions, and letting them do the feature detection for you,
// should provide the most consistent user experience and minimize the amount of work you need to do.
// (Bonus: It also gives us a convenient place to smooth out any future library migrations for you!)
//
// If writing new APIs that are meant to work reusably for any storage implementation:
// APIs should usually be designed around accepting ReadableStorage or WritableStorage as parameters
// (depending on which direction of data flow the API is regarding).
// and use the other interfaces (e.g. StreamingReadableStorage) thereafter internally for feature detection.
// For APIs which may sometimes be found relating to either a read or a write direction of data flow,
// the Storage interface may be used in order to define a function that should accept either ReadableStorage or WritableStorage.
// In other words: when writing reusable APIs, one should follow the same pattern as this package's own functions do.
//
// Similarly, implementers of storage systems should always implement either ReadableStorage or WritableStorage first.
// Only after satisfying one of those should the implementation then move on to further supporting
// additional interfaces in this package (all of which are meant to support feature-detection).
// Beyond one of the basic two, all the other interfaces are optional:
// you can implement them if you want to advertise additional features,
// or advertise fastpaths that your storage system supports;
// but you don't have implement any of those additional interfaces if you don't want to,
// or if your implementation can't offer useful fastpaths for them.
//
// Storage systems as described by this package are allowed to make some interesting trades.
// Generally, write operations are allowed to be first-write-wins.
// Furthermore, there is no requirement that the system return an error if a subsequent write to the same key has different content.
// These rules are reasonable for a content-addressed storage system, and allow great optimizations to be made.
//
// Note that all of the interfaces in this package only use types that are present in the golang standard library.
// This is intentional, and was done very carefully.
// If implementing a storage system, you should find it possible to do so *without* importing this package.
// Because only standard library types are present in the interface contracts,
// it's possible to implement types that align with the interfaces without refering to them.
//
// Note that where keys are discussed in this package, they use the golang string type --
// however, they may be binary.  (The golang string type allows arbitrary bytes in general,
// and here, we both use that, and explicitly disavow the usual "norm" that the string type implies UTF-8.
// This is roughly the same as the practical truth that appears when using e.g. os.OpenFile and other similar functions.)
// If you are creating a storage implementation where the underlying medium does not support arbitrary binary keys,
// then it is strongly recommend that your storage implementation should support being configured with
// an "escaping function", which should typically simply be of the form `func(string) string`.
// Additional, your storage implementation's documentation should also clearly describe its internal limitations,
// so that users have enough information to write an escaping function which
// maps their domain into the domain your storage implementation can handle.
package storage

// also note:
// LinkContext stays *out* of this package.  It's a chooser-related thing.
// LinkSystem can think about it (and your callbacks over there can think about it), and that's the end of its road.
// (Future: probably LinkSystem should have SetStorage and SetupStorageChooser methods for helping you set things up -- where the former doesn't discuss LinkContext at all.)
