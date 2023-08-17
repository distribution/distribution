# Storage

**Status: under development; This is currently just the interface**

A storage extension persists state beyond the collector process. Other components can request a storage client from the storage extension and use it to manage state. 

The `storage.Extension` interface extends `component.Extension` by adding the following method:
```
GetClient(context.Context, component.Kind, config.ComponentID, string) (Client, error)
```

The `storage.Client` interface contains the following methods:
```
Get(context.Context, string) ([]byte, error)
Set(context.Context, string, []byte) error
Delete(context.Context, string) error
Close(context.Context) error
```

It is possible to execute several operations in a single transaction via `Batch`. The method takes a collection of
`Operation` arguments (each of which contains `Key`, `Value` and `Type` properties):
```
Batch(context.Context, ...Operation) error
```

The elements itself can be created using:

```
SetOperation(string, []byte) Operation
GetOperation(string) Operation
DeleteOperation(string) Operation
```

Get operation results are stored in-place into the given Operation and can be retrieved using its `Value` property.

Note: All methods should return error only if a problem occurred. (For example, if a file is no longer accessible, or if a remote service is unavailable.)

Note: It is the responsibility of each component to `Close` a storage client that it has requested.
