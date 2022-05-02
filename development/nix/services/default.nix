{ lib
, callPackage
}:

[
  (callPackage ./postgres.nix { })
  (callPackage ./setup-db.nix { })
]
