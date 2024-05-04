# DateiLager

Dateilager is a content-addressed, networked filesystem for keeping large, similar directories among a wide variety of hosts. It has a few key properties:

- file contents and versions are stored in by a central server, with readers and writers as clients
- files and trees of files are stored only once in a content-addressed store by the server for efficiency, but are still served securely to only clients with access
- readers and writers keep a copy of the filesystem synced locally for very fast local access (and trade off consistency or atomicity)
- incremental updates by readers are O(changes) and optimized to be very fast
- one Dateilager server supports storing many different independent filesystems with secured access to each (multi-tenancy)

Dateilager is used in production to power https://gadget.dev to sync the filesystems for all the apps on the Gadget platform. Dateilager shines at syncing the (often very large) `node_modules` folder shared between the many node.js applications, as well as the comparatively small `.js` files comprising the actual business logic of one particular app.

## Docs

You can read more about DateiLager's design in the `docs/` directory. A good place to start is the [core concepts file](./docs/core.md).

## Contributing

See the development documentation in [Contributing.md](./Contributing.md)
