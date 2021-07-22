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

write_reset_script() {
    local dburi="${1}"
    local file="${HOME}/reset-db.sh"

    log "writing ${file}"

    cat <<EOF > "${file}"
#!/usr/bin/env bash

migrate -path "${HOME}/migrations" -database "${dburi}?sslmode=disable" down -all
migrate -path "${HOME}/migrations" -database "${dburi}?sslmode=disable" up
EOF
    chmod +x "${file}"
}

main() {
    if [[ "$#" -ne 1 ]]; then
        error "Usage: ${0} <dburi>"
    fi
    local dburi="${1}"

    wait_for_postgres "${dburi}"
    write_reset_script "${dburi}"

    if [[ "${RUN_MIGRATIONS:-0}" == "1" ]]; then
        log "run migrations"
        migrate -path "${HOME}/migrations" -database "${dburi}?sslmode=disable" up
    fi

    log "start dateilager server"
    "${HOME}/server" -dburi "${dburi}"
}

main "$@"
