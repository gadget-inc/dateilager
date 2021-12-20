# DateiLager Core Concepts

DateiLager is a shared file system. It allows multiple clients to read and update files, referred to as objects, and it can materialize snapshots of this filesystem directly to disk.

## Use Cases

It was designed to work with Gadget in three different use cases.

### Directly Read and Write Objects

This is how the Gadget API interacts with DateiLager. It can read or list objects within a filesystem, and commit changes to these objects.

These interactions happen over GRPC calls using the included Javascript GRPC client.

The typical use case for a project will look something like this:

- On load, list all objects within a project's filesystem and display them in the sidebar.
- Allow the user to view these objects within the web UI.
- When a user saves a change within the web UI, send the object update to the DateiLager service.

After an object is updated, the API will receive a new version for the project. It can then inform any active instances of that project to update to the latest filesystem version. This leads us directly into our next use case.

### App Pod Filesystem Snapshots

When a pod is spun up to serve a user's app, it will need to get a snapshot of the latest view of the filesystem. DateiLager's included client can be used to rebuild the filesystem state into a local ephmeral drive.

Web requests made to this app will run the code available on this ephmeral drive.

When an update is made within Gadget's web UI, it is Gadget's responsibility to tell all active App pods to update their filesystem state to the latest version.

The included client can be used to apply all updates between it's current filesystem version and the new version sent via Gadget's update mechanism.

### Job Pod Batch Updates

Gadget may choose to run a background task on the latest snapshot of a filesystem. A good example of this is running `npm install` after a user updates the `package.json` within their application.

To make this happen, a new pod will be spun up and the DateiLager client will be used to fetch the latest version of a project's filesystem. Once it's loaded, the background task can be run atop this filesystem snapshot.

Once the background job is complete, we can use the `fsdiff` tool to capture all filesystem changes and then send those as a single batch to the DateiLager service.

This will generate a new filesystem version that Gadget will be responsible for propagating.

## Models

There are 3 main models within DateiLager, which align with the 3 database tables necessary for running the service.

### Projects

A project encapsulates 1 filesystem and all the different versions of that filesystem over time. In Gadget there is a 1 to 1 relationship between a user's environment and a project.

The only details currently tracked about a project in DateiLager are: an ID and it's latest version ID.

### Objects

Objects are the most important model within DateiLager and they can represent 1 of 4 different things:

1. A regular file
2. A symlink
3. An empty directory
4. A packed collection of objects (more on this in the next section)

Every project is a collection of objects.

When an object is first created it is assigned a `start_version` equal to the project's latest version and a `NULL` `stop_version`. This describes a live object, and it remains live up until the point where another update for that same path is received.

At that point, the object's `stop_version` is set to version in which it was no longer valid. And either a new object is created for that path (if the file was updated) or not (if the file was deleted).

Along with versions, objects track their path, size, mode and the hash of the object's contents.

### Contents

This is a content addressable storage table.

It is indexed by the hash of the object's content and contains a compressed version of that content. If two objects, regardless of their project, contain the same content then they will share a row in this table. This is useful as many projects will share common Javascript dependencies.

## Packed Objects

While it is possible to track every single object in a project with their own database row, this quickly gets out of hand as we track Javascript dependencies. Simple Javascript projects can have thousands of files within them, and simple updates to `package.json` can generate modifications to a huge number of objects.

In order to keep the data within Postgres manageable, Gadget can choose to `Pack` certain paths within the filesystem. A good example of a path that may be packed is something like: `node_modules/lodash/*`.

To manage packing rules, when creating a new project you can include a list of pack patterns, these regexes will be used to decide if a path is packed or not.

When updates are made to a packed object, it is decompressed on the fly, the updates are applied and it is then recompressed. This trades off read and write performance to these individual objects to offer a large reduction in total rows within Postgres.

## Filesystem Rebuilds

When either an App or Job pods are initiated, we need to quickly write to their local disk to initiate the filesystem.

To speed up this process, instead of requesting all individual objects, the client can request larger compressed chunks of files. The client can then stream these TAR files and decompress them in parallel.

DateiLager will build these compressed TAR files on the fly, but if any objects within the project are already packed it can forward these directly to the client without any need to recompress.

## Incremental Updates

Once an App pod is up and running, it may receive a notification from Gadget that it should update it's filesystem to a new version.

To avoid rebuilding it's filesystem from scratch, all updates can be done incrementally. This allows it to request only what has changed from it's version to the one it was notified about.

In a typical use case, this will often mean just a small handful of objects need to be updated on disk.
