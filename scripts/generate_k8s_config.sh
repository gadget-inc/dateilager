#!/usr/bin/env bash
# shellcheck disable=SC2155

set -euo pipefail

log() {
    echo "$(date +"%H:%M:%S") - $(printf '%s' "$@")" 1>&2
}

error() {
    local message="${1}"

    echo "$(date +"%H:%M:%S") - ERROR: $(printf '%s' "${message}")" >&2
    exit 55
}

main() {
    if [[ "$#" -ne 1 ]]; then
        error "Usage: ${0} <dburi>"
    fi

    log "writing k8s configs to k8s/server.properties"
    local dburi="${1}"

    cat <<- EOF > k8s/server.properties
port=5051
dburi=${dburi}
env=dev
	EOF
}

main "$@"
