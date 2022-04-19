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
    ]
      (system: nixpkgs.lib.fix (flake:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          callPackage = pkgs.newScope (flake.packages // { inherit callPackage; });
          services = callPackage ./development/nix/services { };
        in
        rec {
          packages = rec {
            ## DateiLager development scripts

            clean = callPackage ./development/nix/scripts/clean.nix { };

            dev = callPackage ./development/nix/scripts/dev.nix {
              inherit services;
            };

            ## Packages from nixpkgs

            mkcert = pkgs.mkcert;

            go = pkgs.go_1_17;

            git = pkgs.git;

            nodejs = pkgs.nodejs-16_x;

            protobuf = pkgs.protobuf;

            postgresql = pkgs.postgresql_13;
          };

          devShell = pkgs.mkShell {
            packages = builtins.attrValues self.packages.${system};
            shellHook = ''
              GOROOT=${pkgs.go_1_17}/share/go
            '';
          };
        }
      )));
}
