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

ensure_db() {
    local dburi="${1}"
    local dbname="${2}"

    if [[ "$(psql "${dburi}" -tAc "SELECT 1 FROM pg_database WHERE datname = '${dbname}'")" != "1" ]]; then
        log "create DB ${dbname}"
        psql "${dburi}" -c "CREATE DATABASE ${dbname};" > /dev/null
    fi
}

write_reset_script() {
    local appdb="${1}"
    local file="${HOME}/reset-db.sh"

    log "writing ${file}"

    cat <<EOF > "${file}"
#!/usr/bin/env bash

migrate -path "${HOME}/migrations" -database "${appdb}?sslmode=disable" down -all
migrate -path "${HOME}/migrations" -database "${appdb}?sslmode=disable" up
EOF
    chmod +x "${file}"
}

main() {
    if [[ "$#" -ne 3 ]]; then
        error "Usage: ${0} <port> <dburi> <dbname>"
    fi
    local port="${1}"
    local dburi="${2}"
    local dbname="${3}"

    local rootdb="${dburi}/postgres"
    local appdb="${dburi}/${dbname}"

    wait_for_postgres "${rootdb}"
    ensure_db "${rootdb}" "${dbname}"

    write_reset_script "${appdb}"

    if [[ "${RUN_MIGRATIONS:-0}" == "1" ]]; then
        log "run migrations"
        migrate -path "${HOME}/migrations" -database "${appdb}?sslmode=disable" up
    fi

    local log_level="${DL_LOG_LEVEL:-info}"
    local secrets="${HOME}/secrets"

    log "start dateilager server"
    "${HOME}/server" -dburi "${appdb}" -port "${port}" -log "${log_level}" -cert "${secrets}/server.crt" -key "${secrets}/server.key" -paseto "${secrets}/paseto.pub"
}

main "$@"
