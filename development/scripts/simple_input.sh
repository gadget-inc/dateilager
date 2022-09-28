#!/usr/bin/env bash
# shellcheck disable=SC2155

set -euo pipefail

log() {
    echo "$(date +"%H:%M:%S") - $(printf '%s' "$@")" 1>&2
}

realpath() {
    local path="${1}"
    echo "$(cd ${path}; pwd -P)"
}

readonly ROOT_DIR="$(realpath "$(dirname "$0")/../..")"
readonly INPUT_DIR="${ROOT_DIR}/input/simple"

v1() {
    local dir="${1}"

    rm -rf "${dir:?}"
    mkdir -p "${dir}"

    echo "a" > "${dir}/a"
    echo "b" > "${dir}/b"
    echo "c" > "${dir}/c"

    mkdir -p "${dir}/n1/n2"
    echo "g" > "${dir}/n1/g"

    log "wrote v1 to ${dir}"
}

v2() {
    local dir="${1}"

    echo "d" > "${dir}/d"
    ln -s a "${dir}/e"
    mkdir -p "${dir}/f"
    rm "${dir}/c"

    (
        cd "${dir}/n1/n2"
        ln -s "../g" h
    )

    log "wrote v2 to ${dir}"
}

v3() {
    local dir="${1}"

    echo "e" > "${dir}"/a
    rm "${dir}/b"

    log "wrote v3 to ${dir}"
}

main() {
    if [[ $# -ne 1 ]]; then
        log "missing necessary version argument"
        exit 1
    fi

    log "writing simple inputs to ${INPUT_DIR}"

    case "$1" in
        "1") v1 "${INPUT_DIR}" ;;
        "2") v2 "${INPUT_DIR}" ;;
        "3") v3 "${INPUT_DIR}" ;;
        *) log "invalid version argument"; exit 1 ;;
    esac

    log "wrote files"
}

main "$@"
