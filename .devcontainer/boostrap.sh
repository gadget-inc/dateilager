#!/usr/bin/env bash

# source the nix environment
source /home/vscode/.nix-profile/etc/profile.d/nix.sh

set -euox pipefail

# fixes nix builds within github codespaces, see https://github.com/xtruder/nix-devcontainer/issues/12
sudo setfacl --remove-default  /tmp

sudo locale-gen en_US.UTF-8
cp .vscode/settings.example.json .vscode/settings.json

# install cachix ahead of time so that the first load of the devshell is snappier
nix profile install nixpkgs#cachix
cachix authtoken $(cat development/cachix-auth-token)
cachix use gadget-core

# Pre-load development environment
direnv allow
nix print-dev-env development/ > /dev/null
