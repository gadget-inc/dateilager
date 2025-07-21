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

### Getting PASETO tokens locally

You can sign PASETO tokens locally with this handy online tool: https://token.dev/paseto/. Ensure you use the V2 algorithm in the public mode, and copy the PASTEO public and private key from the `development` folder.

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

## Developing Cached

Dateilager has a CSI driver named cached (cache daemon) that is deployed as a Kubernetes DaemonSet. This driver is responsible for preparing a `dl_cache` and mounting it on Kubernetes pods so that future `dateilager-client rebuild` commands can use it.

Cached uses [Logical Volume Manager (LVM)](https://www.man7.org/linux/man-pages/man8/lvm.8.html) to manage mounting the `dl_cache` volume on Kubernetes pods. LVM is only available on Linux, so cached can only be developed and deployed on Linux.

Here are steps to setup a development environment on a GCP VM that can be used to develop cached. This guide assumes you want to use VSCode/Cursor's Remote-SSH feature.

1. Create GCP VM running Debian with a 100GB boot disk so we have enough space to install nix and other dependencies.

   If you want to run benchmarks, it's best to use the same configuration as your production Kubernetes nodes, e.g. `n2d-standard-16` with a 500GB `pd-balanced` disk for the base PV and local SSD(s) for the thinpool PVs.
1. SSH into the VM so gcloud knows where about it.
   ```
   gcloud compute ssh --zone "us-central1-a" <your-vm-name> --project <your-project-id>
   ```
1. Setup SSH config so the VM shows up in VSCode/Cursor's Remote-SSH list.
   ```
   gcloud compute config-ssh
   ```
1. Run `Remote-SSH: Connect to Host...` in VSCode/Cursor to connect to the VM. Now you can run the rest of the commands using VSCode/Cursor's terminal.
1. Install OS packages needed for development.
   ```
   sudo apt update
   sudo apt install -y direnv git lvm2 sysstat xfsprogs
   sudo modprobe -v xfs
   ```
1. Install Golang so VSCode/Cursor can use it to run the LSP server.
   ```
   wget https://go.dev/dl/go1.24.5.linux-amd64.tar.gz
   rm -rf /usr/local/go
   sudo tar -C /usr/local -xzf go1.24.5.linux-amd64.tar.gz
   ```
1. Install Nix.
   ```
   sh <(curl --proto '=https' --tlsv1.2 -L https://nixos.org/nix/install) --daemon
   ```
1. Update `/etc/nix/nix.conf` to enable nix-command and flakes.
   ```
   cat <<EOF | sudo tee /etc/nix/nix.conf
   build-users-group = nixbld
   experimental-features = nix-command flakes
   trusted-users = root $USER
   EOF
   ```
1. Update `.bashrc` to setup Go for VSCode/Cursor's LSP server and Nix and direnv for interactive shells.

   ```
   # Add go to PATH so it's available to all shells (including remote-ssh)
   sed -i '/^# If not running interactively, don'\''t do anything$/i export PATH=/usr/local/go/bin:$PATH' ~/.bashrc

   # Setup Nix and direnv for interactive shells
   cat << 'EOF' >> ~/.bashrc
   if ! which nix; then
      unset __ETC_PROFILE_NIX_SOURCED
   fi

   if [ -e '/nix/var/nix/profiles/default/etc/profile.d/nix-daemon.sh' ]; then
      . '/nix/var/nix/profiles/default/etc/profile.d/nix-daemon.sh'
   fi

   eval "$(direnv hook bash)"
   EOF
   ```

1. Reinstall Remote Server to pick up the Go LSP server changes.
   ```
   Remote-SSH: Reinstall Remote Server and Reload Window
   ```
1. Clone the dateilager repo and setup direnv.
   ```
   mkdir -p ~/repos/dateilager
   cd ~/repos/dateilager
   git clone https://github.com/gadget-inc/dateilager.git .
   direnv allow
   ```
1. If you didn't attach any disks to the VM, you can setup some loop devices to use for the base PV and thinpool PVs.

   ```
   mkdir -p tmp/img
   truncate -s 5G tmp/img/lvm-base.img
   truncate -s 5G tmp/img/lvm-thin1.img
   truncate -s 5G tmp/img/lvm-thin2.img

   sudo losetup --sector-size 4096 --direct-io=on --find --show tmp/img/lvm-base.img
   sudo losetup --sector-size 4096 --direct-io=on --find --show tmp/img/lvm-thin1.img
   sudo losetup --sector-size 4096 --direct-io=on --find --show tmp/img/lvm-thin2.img
   ```

1. Export these environment variables so cached knows where the base PV and thinpool PVs are.

   ```
   # Use loop devices
   export DL_BASE_PV=/dev/loop0
   export DL_THINPOOL_PV_GLOBS=/dev/loop1,/dev/loop2

   # OR

   # Use real devices
   export DL_BASE_PV=/dev/disk/by-id/google-dl-testing
   export DL_THINPOOL_PV_GLOBS=/dev/disk/by-id/google-local-nvme-ssd-*
   ```

That's it! You can now start developing cached.

```bash
# in one tab, run the background services (postgres)
dev

# in another tab, run integration tests that test the cached driver
make test-integration
```

### Preparing the base PV for production

Cached works best with a large `dl_cache`. When cached starts up, it will download the `dl_cache` and prepare the base PV if it's not already prepared. This is great for development and testing, but bad for production because the base PV takes longer to prepare the larger the `dl_cache` is.

The best way to deploy cached to production is to prepare the base PV on a temporary disk and export it into a GCP image. The exported image can then be attached to Kubernetes nodes as a secondary boot disk so that the disk is immediately available for use when the node boots.

To prepare the base PV for production, first setup a GCP VM using [the instructions above](#developing-cached) and ensure the GCP VM is configured with an additional disk that will be used as the base PV. Assuming the disk is named `dl-cache-base-pv`, you can prepare the base PV by running the following:

```bash
# ensure cached is built
make bin/cached

# prepare the base PV
DL_TOKEN='<your-token>' sudo -E env PATH="/usr/sbin:$PATH" bin/cached prepare \
    --base-pv=/dev/disk/by-id/google-dl-cache-base-pv \
    --upstream-host='<your-upstream-host>' \
    --cache-version='<your-cache-version>'
```

Once the base PV is prepared, you can resize the base LV to have a specified amount of free space. This is useful because snapshots inherit the free space of the base LV.

```bash
# activate the base LV and make it writable
sudo lvchange -ay -p rw vg_dl_cache/base

# resize the base LV to have 12GB of free space
sudo development/scripts/cached-resize.sh 12

# deactivate the base LV and make it read-only
sudo lvchange -an -p r vg_dl_cache/base
```

Once the base PV is prepared, you can create a GCP image using the prepared disk as the source.

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

### Pre-release

When you want to test your changes before they've been released - even before they've been merged into `main` - you can create a pre-release.

First, ensure you working directory is clean and the current commit has been pushed.

```bash
git status
git push origin HEAD
```

Then, create a pre-release:

```bash
make prerelease
```

This will create a pre-release version named `0.0.0-pre.<git-sha>` and:

1. Build and push the docker image to Gadget's container registry
2. Tag the current commit with the pre-release version and push the tag to GitHub
3. Publish the pre-release js package to GitHub

You can then test the pre-release in Gadget's repo using the `update-dateilager.ts` script.

```bash
bin/update-dateilager.ts v0.0.0-pre.<git-sha>
```
