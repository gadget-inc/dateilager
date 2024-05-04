{
  description = "DateiLager development environment";

  inputs = {
    flake-utils.url = "github:numtide/flake-utils";
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
  };

  outputs = { self, flake-utils, nixpkgs }:
    (flake-utils.lib.eachSystem [
      "x86_64-linux"
      "x86_64-darwin"
      "aarch64-darwin"
      "aarch64-linux"
    ]
      (system: nixpkgs.lib.fix (flake:
        let
          pkgs = nixpkgs.legacyPackages.${system};

          lib = pkgs.lib // {
            maintainers = pkgs.lib.maintainers // {
              angelini = {
                name = "Alex Angelini";
                email = "alex.louis.angelini@gmail.com";
                github = "angelini";
                githubId = 515110;
              };
              scott-rc = {
                name = "Scott Côté";
                email = "scott.cote@hey.com";
                github = "scott-rc";
                githubId = 21965521;
              };
            };
          };

          callPackage = pkgs.newScope (flake.packages // { inherit lib callPackage; });
        in
        {
          packages =  {
            ## DateiLager development scripts

            clean = callPackage ./development/nix/scripts/clean.nix { };

            dev = callPackage ./development/nix/scripts/dev.nix {
              services = callPackage ./development/nix/services { };
            };

            ## Pinned packages from nixpkgs

            go = pkgs.go_1_22;

            nodejs = pkgs.nodejs-18_x;

            postgresql = pkgs.postgresql_14;

            ## DateiLager outputs

            dateilager = callPackage ./. {
              buildGoModule = pkgs.buildGoModule.override {
                go = flake.packages.go;
              };
            };
          };

          defaultPackage = flake.packages.dateilager;

          devShell = pkgs.mkShell {
            buildInputs = with pkgs; [
              flake.packages.go
              flake.packages.nodejs
              flake.packages.postgresql
              flake.packages.dev
              flake.packages.clean
              git
              protobuf
              protoc-gen-go
              protoc-gen-go-grpc
              go-migrate
              mkcert
            ];

            shellHook = ''
              # prepend the built binaries to the $PATH
              export PATH="./bin":$PATH
          '';
          };
        }
      )));
}
