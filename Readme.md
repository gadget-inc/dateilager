# DateiLager

## Setup

### Requirements

- Go 1.15
- Postgresql

Create a Postgres database named `dl`. The default Postgres host is `127.0.0.1` you can override it by
exporting `DB_HOST`.

```bash
$ export DB_HOST=10.0.0.1
```

### Install Go Tools

Ensure that you have a working Go development environment: https://golang.org/doc/install

And that you are running at least Go 1.15.

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

Import the `DateiLagerClient` from `js/client.js` and use it to query objects:

```javascript
// A client for project 1
const client = new DateiLagerClient("localhost", 5051, 1);

// List all objects
const listStream = client.listObjects("")

listStream.on("data", function (data) {
  const output = [
    "version: " + data.version,
    "path: " + data.object.path,
    "size: " + data.object.size,
    "mode: " + data.object.mode,
  ];
  console.log("[listObjects] OBJECT: " + output.join(", "));
  console.log(
    "[listObjects] CONTENT:\n" + data.object.content.toString("utf-8")
  );
});

listStream.on("error", function (error) {
  console.log("[listObjects] ERROR: " + error);
});

listStream.on("end", function () {
  console.log("[listObjects] END");
});
```

Update objects, closing the stream will increment the project's version and commit the updates:

```javascript
const updateStream = client.updateObjects(function (err, stats) {
  if (err) {
    console.log("[updateObjects] ERROR: " + err);
    return;
  }

  console.log("[updateObjects] VERSION: " + stats.version);
});

updateStream.write({
  path: "c",
  mode: 420,
  content: Buffer.from("c v4"),
});

updateStream.end();
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

## References

- https://github.com/golang-standards/project-layout
