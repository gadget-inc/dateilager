# DateiLager

## Docs

You can read more about DateiLager's design in the `docs/` directory. A good place to start is the [core concepts file](./docs/core.md).

## Setup

### Requirements

- Go 1.16
- Postgresql
- Node v15
- Protobuf Compiler (https://grpc.io/docs/protoc-installation/)

Create a Postgres database named `dl`. The default Postgres host is `127.0.0.1` you can override it by
exporting `DB_HOST`.

```bash
$ export DB_HOST=10.0.0.1
```

### Install Go Tools

Ensure that you have a working [Go development environment](https://golang.org/doc/install) and that you are running at least Go 1.16.

You will also require `npm`.

```bash
$ make install
```

### Build

This will build the server and client executables along with the `*.proto` files.

```bash
$ make build
```

## API Testing

Ensure there is a Postgres database named `dl_tests`. These tests will write to a real database instance
but all writes will be rolled back as every test runs within it's own transaction.

```bash
$ make test
```

## Local

Set up a local development environment by resetting the local database, and building an example dataset
within the `input/` directory.

```bash
$ make setup-local
```

Then launch the server process, by default it will run on port 5051.

```
$ make server
```

Our input directory contains 3 folders, all meant as different versions of the same project. There are also
diff files listing which files have changed between them.

We can now load these into the database.

```bash
$ make client-update
```

And then use the client app to read all files within the latest version.

```bash
$ make client-get
```

You can also filter the results with a prefix search.

```bash
$ make client-get prefix=/a
```

Or filter for a specific version.

```bash
$ make client-get version=1
```

If you want to rebuild an entire project's directory locally, use the `rebuild` command.

```bash
$ mkdir ./rebuild
$ make client-rebuild version=3 prefix=/ output=rebuild
```

## Javascript Client

Ensure a server is running with `make server`.

Import the `DateiLagerClient` from the module in `js/` and use it to query objects:

```javascript
const client = new DateiLagerClient("localhost", 5051);

// Get a single object
const object = await client.getObject(1n, "a");
console.log("[getObject] path: " + object.path);
console.log("[getObject] content:\n" + object.content);

// List all objects
for await (const object of client.listObjects(1n, "")) {
  console.log("[listObjects] path: " + object.path);
  console.log("[listObjects] content:\n" + object.content);
}
```

Update objects and await the successful commit of a new version:

```javascript
const stream = client.updateObjects(1n);

stream.send({
  path: "a",
  mode: 0o755,
  content: "foo bar",
});

const version = await stream.complete();
console.log("[updateObject] version: " + version);
```

## Release

New versions are released and hosted on Github. (https://github.com/gadget-inc/dateilager/releases)

As a first step build all the files necessary for a release.

```bash
$ make release
```

Then create a new git tag and push it to Github.

```bash
$ git tag v0.0.x
$ git push origin v0.0.x
```

Then navigate to the release page (https://github.com/gadget-inc/dateilager/releases/edit/v0.0.x), add a description and upload all files found in the `release` directory.

Finally we need to build and push the Docker image to it's repository.

```bash
docker build -t gcr.io/gadget-core-production/dateilager:0.0.x .
docker push gcr.io/gadget-core-production/dateilager:0.0.x
```

## K8S

The K8S tools assume a local K8S install using Containerd and Podman.

### Requriements

- kubectl
- podman
- ctr

### Build

This is meant for local development, it will reset the `dateilager` namespace.

```bash
$ make k8s
```

### Client

Execute the client locally and have it connect to a server in K8S. All of the same `client-*` make commands are supported
but they require a `k8s-` prefix.

```bash
$ make k8s-client-get
```
