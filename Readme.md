# DateiLager

## Docs

You can read more about DateiLager's design in the `docs/` directory. A good place to start is the [core concepts file](./docs/core.md).

## Setup

### Requirements

- Go 1.18
- Postgresql
- Node v16
- mkcert (https://github.com/FiloSottile/mkcert)
- Protobuf Compiler (https://grpc.io/docs/protoc-installation/)

Create a Postgres database named `dl`. The default Postgres host is `127.0.0.1` you can override it by
exporting `DB_HOST`.

```bash
$ export DB_HOST=10.0.0.1
```

### Install Go Tools

Ensure that you have a working [Go development environment](https://golang.org/doc/install) and that you are running at least Go 1.18.

You will also require `npm`.

```bash
$ make install
```

### Setup VSCode (Optional)

We recommend using VSCode for development, and there's an example settings file at `.vscode/settings.example.json` to get started with:

```
cp .vscode/settings.example.json .vscode/settings.json
```

#### Install `golangci-lint`

```
brew tap golangci/tap
brew install golangci/tap/golangci-lint
```

### Build

This will build the server and client executables along with the `*.proto` files.

```bash
$ make build
```

### Lint

This will lint the whole project and report any problems:

```bash
$ make lint
```

## API Testing

Ensure there is a Postgres database named `dl_tests`. These tests will write to a real database instance
but all writes will be rolled back as every test runs within its own transaction.

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
$ make client-get prefix=n1
```

Or filter for a specific version.

```bash
$ make client-get to_version=1
```

If you want to rebuild an entire project's directory locally, use the `rebuild` command.

```bash
$ make client-rebuild to_version=3 dir=./rebuild
$ ls -lah ./rebuild
```

## Javascript Client

Ensure a server is running with `make server`.

Import the `DateiLagerGrpcClient` from the module in `js/` and use it to query objects:

```javascript
const client = new DateiLagerGrpcClient("localhost", 5051);

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

Create a new tag and push it to GitHub, GoReleaser will handle building it.

```bash
$ git tag v0.0.x
$ git push origin v0.0.x
```

We also need to build the server docker image and push it to Gadget's container registry.

```bash
$ make upload-container-image version=0.0.x
```

## K8S

The K8S tools assume a local K8S install using Containerd and Podman.

### Requirements

- docker
- kubectl
- ctr

### Build

This is meant for local development, it will reset the `dateilager` namespace.

```bash
$ make k8s
```

### Client

Execute the client locally and have it connect to a server in K8S. All the same `client-*` make commands are supported,
but they require a `k8s-` prefix.

```bash
$ make k8s-client-get
```
