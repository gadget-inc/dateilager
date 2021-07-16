#!/usr/bin/env bash

set -euo pipefail

log() {
    echo "$(date +"%H:%M:%S") - $(printf '%s' "$@")" 1>&2
}

error() {
    local message="${1}"

    echo "$(date +"%H:%M:%S") - ERROR: $(printf '%s' "${message}")" >&2
    exit 55
}

wait_for_postgres() {
    local dburi="${1}"

    log "wait for postgres"

    until psql "${dburi}" -c '\q' 2> /dev/null; do
        sleep 1
        log "postgres not ready"
    done
}

main() {
    if [[ "$#" -ne 1 ]]; then
        error "Usage: ${0} <dburi>"
    fi
    local dburi="${1}"

    wait_for_postgres "${dburi}"

    if [[ "${RUN_MIGRATIONS:-0}" == "1" ]]; then
        log "run migrations"
        migrate -path "${HOME}/migrations" -database "${dburi}?sslmode=disable" up
    fi

    log "start dateilager server"
    "${HOME}/server" -dburi "${dburi}"
}

main "$@"
