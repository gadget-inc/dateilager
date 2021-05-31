# DateiLager

## Setup

### Requirements

- Go 1.15
- Postgresql

Create a Postgres database named `dl`. The default Postgres host is `127.0.0.1` you can override it by
exporting `DB_HOST`.

```
$ export DB_HOST=10.0.0.1
```

### Install Go Tools

Ensure that you have a working Go development environment: https://golang.org/doc/install

And that you are running at least Go 1.15.

```
$ make install
```

### Build

This will build the server and client executables along with the `*.proto` files.

```
$ make build
```

## API Testing

Ensure there is a Postgres database named `dl_tests`. These tests will write to a real database instance
but all writes will be rolled back as every test runs within it's own transaction.

```
$ make test
```

## Local

Launch the local server process.

```
$ make server
```

And in a different shell, launch the client.

```
$ make client
```

## K8S

The K8S tools assume a local K8S install using Containerd and Podman.

### Requriements

- kubectl
- podman
- ctr

### Build

This is meant for local development, it will **delete existing services and pods**.

```
$ make k8s
```

### Client

Execute the client locally and have it connect to a server in K8S.

```
$ make client-k8s
```

## References

- https://github.com/golang-standards/project-layout
