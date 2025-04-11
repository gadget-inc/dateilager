## Contributing to Dateilager

TL;DR:

```bash
# allow nix to do its thing
direnv allow

# in one tab, run the background services
dev

# in another, run the migrations, then start developing!
make migrate
make server
```

### Requirements

Dateilager has a fully managed development environment ready for use via `nix`. To use this environment, ensure you have nix and `direnv` installed, and then run `direnv allow` to have the environment build itself.

#### Manual environment

If you don't want to use nix, you can install the components manually:

- Go 1.22
- Postgresql
- Node v20
- mkcert (https://github.com/FiloSottile/mkcert)
- Protobuf Compiler (https://grpc.io/docs/protoc-installation/)

Ensure that you have a working [Go development environment](https://golang.org/doc/install) and that you are running at least Go 1.22. You will also require `npm`.

### Run background services

DL requires a running postgres instance in the background for development. The `dev` script will setup and run this database.

Run

```bash
dev
```

The default Postgres host is `127.0.0.1` you can override it by exporting `DB_HOST`.

```bash
export DB_HOST=10.0.0.1
```

### Setup VSCode (Optional)

We recommend using VSCode for development, and there's an example settings file at `.vscode/settings.example.json` to get started with:

```
cp .vscode/settings.example.json .vscode/settings.json
```

### Build

This will build the server and client executables along with the `*.proto` files.

```bash
make build
```

### Run migrations

Assuming Postgres is running, you can setup the database using the migrate tool:

```
make migrate
```

## API Testing

Ensure there is a Postgres database named `dl_tests`. These tests will write to a real database instance
but all writes will be rolled back as every test runs within its own transaction.

```bash
make test
```

## Local

Set up a local development environment by resetting the local database, and building an example dataset
within the `input/` directory.

```bash
make setup-local
```

Then launch the server process, by default it will run on port 5051.

```
make server
```

Our input directory contains 3 folders, all meant as different versions of the same project. There are also
diff files listing which files have changed between them.

We can now load these into the database.

```bash
make client-update
```

And then use the client app to read all files within the latest version.

```bash
make client-get
```

You can also filter the results with a prefix search.

```bash
make client-get prefix=n1
```

Or filter for a specific version.

```bash
make client-get to_version=1
```

If you want to rebuild an entire project's directory locally, use the `rebuild` command.

```bash
make client-rebuild to_version=3 dir=./rebuild
ls -lah ./rebuild
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

When you're ready to release a new version, perform the following steps. Ensure you are on doing these steps on the `main` branch

1. Update the version in `default.nix`
2. Update the version in `js/package.json`
3. Update the version in `js/package-lock.json` (run `cd js && npm install`)
4. Commit the changes (e.g. `git commit -am "Bump version to 0.0.x"`)
5. Push the changes upstream to main (e.g. `git push origin HEAD`)

New versions are released and hosted on Github. (https://github.com/gadget-inc/dateilager/releases)

Create a new tag and push it to GitHub, GoReleaser will handle building it.

```bash
git tag v0.0.x
git push origin v0.0.x
```

We also need to build the server docker image and push it to Gadget's container registry.

If you haven't already, make sure you're using docker buildx for multi-arch builds.

```bash
docker buildx create --name mybuilder --use
docker buildx install
```

Then build and push the image:

```bash
make upload-container-image version=v0.0.x
```

### Getting PASETO tokens locally

You can sign PASETO tokens locally with this handy online tool: https://token.dev/paseto/. Ensure you use the V2 algorithm in the public mode, and copy the PASTEO public and private key from the `development` folder.
