## Build & Development

Nix-managed dev environment. Run `direnv allow` to activate. Run `dev` to start background services (postgres).

- `make build` — builds all binaries (`server`, `client`, `cached`) and regenerates protobuf code
- `make server` — runs dev server on port 5051 with `DL_ENV=dev`
- `make migrate` — runs DB migrations against `$DB_URI`
- `make migrate-create name=<name>` — creates a new migration file

---

## Testing

Tests live in `test/` (not alongside source files). Each test wraps in a rolled-back transaction via `internal/testutil.TestCtx` — no persistent test data.

- `make test` — Go unit tests against `dl_tests` database
- `make test-one name=TestFunctionName` — single test by name
- `make test-integration` — integration tests requiring sudo/LVM (Linux only)
- `make bench` — benchmarks
- `make test-js` — JS client tests (requires running server)
- `make test-fuzz` — fuzzing stress tests

Tests create in-process gRPC servers via `bufconn` — no network socket needed. Helper functions in `test/shared_test.go` set up projects/objects/content directly in the DB.

---

## Linting

- `make lint` — Go linting via golangci-lint
- `make lint-js` — JavaScript linting

---

## Code Generation

Protobuf source: `internal/pb/fs.proto`. Run `make build` after modifying it — regenerates:
- `internal/pb/fs.pb.go` and `internal/pb/fs_grpc.pb.go` via `protoc`
- `js/src/pb/` via the protobuf-ts plugin

---

## Architecture

Three binaries:
- `server` — gRPC API + PostgreSQL backend
- `client` — CLI for sync operations
- `cached` — Kubernetes CSI driver using LVM for cache management (Linux only)

### Package Layout

- `pkg/api/` — gRPC service implementation (`Fs` service: Get, Update, Rebuild, GC, etc.)
- `internal/db/` — PostgreSQL queries, schema operations, content encoding, garbage collection
- `internal/testutil/` — `TestCtx` wraps DB transactions for automatic rollback
- `test/` — all Go tests
- `js/` — TypeScript/Node.js gRPC client npm package
- `migrations/` — sequential SQL migrations

### Data Model

Four PostgreSQL tables in the `dl` schema (see `migrations/` for full schema):

- `projects` — filesystem containers with versioning and pack patterns
- `objects` — versioned file/symlink/dir entries
- `contents` — content-addressed compressed storage
- `cache_versions` — precomputed cache manifests for packed objects

### Key Concepts

**Versioning** — objects have `start_version` and `stop_version`. Updates set `stop_version` on old objects and insert new ones. Clients request deltas between two versions.

**Packed objects** — regex-driven packing of high-churn paths (e.g. `node_modules/.*/`) into single TAR-compressed DB rows to reduce row count. Controlled per-project via `pack_patterns`.

**Content-addressed storage** — identical files share `contents` rows via SHA256 hash.

**Streaming rebuilds** — compressed TAR streams for fast disk materialization. Packed objects forward directly without recompression.

**`Objekt` type** — the protobuf message is named `Objekt` (not `Object`) to avoid collision with TypeScript's `Object` type. Use this name throughout Go and generated TS code.

### Authentication

Three roles: `Admin` (full access), `Project` (per-project), `SharedReader` (read-only shared cache). Dev keys: `development/paseto.{pub,key}`.

### Environment Variables

- `DL_MAX_MESSAGE_SIZE_MB` — max gRPC message size (default: 400)
- `DL_SKIP_SSL_VERIFICATION` — disable TLS verification (dev only)
- `DL_USE_REFLINKS` — enable reflink copies where supported
