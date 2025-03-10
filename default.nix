{ lib
, buildGoModule
, dev
, git
, nodejs
, protobuf
, protoc-gen-go
, protoc-gen-go-grpc
, go-migrate
, mkcert
, postgresql
, clean
}:

buildGoModule rec {
  pname = "dateilager";
  version = "0.9.1-pre.5854f6c";
  src = ./.;
  proxyVendor = true; # Fixes: cannot query module due to -mod=vendor running make install
  vendorHash = "sha256-K1iMGDetMcC1qi+IQ9TkPSMhKMDv4TvOLzVs5Fj3eZo=";

  outputs = [ "out" "client" "server" "migrations" ];

  nativeBuildInputs = [
    git
    nodejs
    protobuf
    protoc-gen-go
    protoc-gen-go-grpc

    # TODO: Figure out a way to only add this for `nix develop`, this isn't needed to build
    clean
  ] ++ checkInputs; # Workaround to get checkInputs loaded in `nix develop` - should be fixed upstream in Nix

  # TODO: make js/src/fs.client.ts, requires node dependencies to be nixified
  preBuild = ''
    make internal/pb/fs.pb.go internal/pb/fs_grpc.pb.go
  '';

  ldflags = [
    "-X"
    "github.com/gadget-inc/dateilager/pkg/version.Version=v${version}"
  ];

  # Postgres fails to start in Nix sandbox with:
  # FATAL:  could not create shared memory segment: Operation not permitted
  doCheck = false;

  checkInputs = [
    dev
    go-migrate
    mkcert
    postgresql
  ];

  __darwinAllowLocalNetworking = true;

  preCheck = ''
    export HOME=$(mktemp -d)
    make development/server.crt
    dev &
    wait_for_postgres
  '';

  postInstall = ''
    mkdir -p "$client/bin" "$server/bin"
    mv "$out/bin/client" "$client/bin/dateilager-client"
    mv "$out/bin/server" "$server/bin/dateilager-server"
    cp -r ${./migrations} "$migrations"
    ln -s "$client/bin/dateilager-client" "$server/bin/dateilager-server" "$out/bin"
  '';

  meta = with lib; {
    description = "A shared file system";
    homepage = "https://github.com/gadget-inc/dateilager";
    license = licenses.free;
    maintainers = with maintainers; [ angelini kira-bruneau scott-rc ];
    mainProgram = "dateilager-server";
  };
}
