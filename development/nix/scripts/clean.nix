{ lib
, writeShellScriptBin
, git
}:

let
  keepFiles = [
    # Local-specific files
    ".envrc.local"
    ".vscode"
    "TODO"

    # Cache files
    "node_modules" # Has no functional effect on `dev` service since new/updated modules will be fetched when needed
  ];
in
writeShellScriptBin "clean" ''
  ${git}/bin/git clean -xdf ${lib.concatStringsSep " " (builtins.map (file: "-e ${file}") keepFiles)}
'' // {
  meta = {
    description = "Reset DateiLager back to a clean state as if it was cloned for the first time";
  };
}
