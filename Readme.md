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

Set up a local project by inserting one into the `projects` table.

```sql
INSERT INTO dl.projects (id, latest_version)
VALUES (1, 0);
```

Launch the local server process.

```
$ make server
```

For testing, we'll need 3 different local folders which are different versions of the same project.

```bash
$ mkdir -p input/v1
$ echo "a" > input/v1/a
$ echo "b" > input/v1/b

$ cp -r input/v1 input/v2
$ echo "c" > input/v2/c

$ cp -r input/v2 input/v3
$ echo "d" > input/v3/a
$ echo "e" > input/v3/b
```

Create an initial list of files and diffs between versions.

```bash
$ find input/v1 -type f > input/initial.txt
$ git diff --name-only --no-index --diff-filter=d -l0 input/v1 input/v2 > input/diff_v1_v2.txt
$ git diff --name-only --no-index --diff-filter=d -l0 input/v2 input/v3 > input/diff_v2_v3.txt
```

We can use these diffs and the client application to load these 3 different versions.

```bash
$ make client-update file=input/initial.txt prefix=input/v1
$ make client-update file=input/diff_v1_v2.txt prefix=input/v2
$ make client-update file=input/diff_v2_v3.txt prefix=input/v3
```

And then use the client app to read the latest version.

```bash
$ make client-get
```

## K8S

The K8S tools assume a local K8S install using Containerd and Podman.

### Requriements

- kubectl
- podman
- ctr

### Build

This is meant for local development, it will **delete existing services and pods**.

```bash
$ make k8s
```

### Client

Execute the client locally and have it connect to a server in K8S.

```bash
$ make client-k8s
```

## References

- https://github.com/golang-standards/project-layout
