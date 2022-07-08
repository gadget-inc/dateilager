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
readonly INPUT_DIR="${ROOT_DIR}/input/complex"

build_node_modules() {
    local package_json="${1}"
    local output="${2}"

    mkdir -p "${output}/node_modules"

    cp "${package_json}" "${output}/package.json"
    (
        cd "${output}"
        npm install &> /dev/null
    )
}

v1() {
    local dir="${1}"
    rm -rf "${dir:?}"
    mkdir -p "${dir}"
    build_node_modules "${ROOT_DIR}/development/scripts/package-v1.json" "${dir}"
    log "wrote v1 to ${dir}"
}

v2() {
    local dir="${1}"
    build_node_modules "${ROOT_DIR}/development/scripts/package-v2.json" "${dir}"
    log "wrote v2 to ${dir}"
}

v3() {
    local dir="${1}"
    build_node_modules "${ROOT_DIR}/development/scripts/package-v3.json" "${dir}"
    log "wrote v3 to ${dir}"
}

main() {
    if [[ $# -ne 1 ]]; then
        log "missing necessary version argument"
        exit 1
    fi

    log "writing complex inputs to ${INPUT_DIR}"

    case "$1" in
        "1") v1 "${INPUT_DIR}" ;;
        "2") v2 "${INPUT_DIR}" ;;
        "3") v3 "${INPUT_DIR}" ;;
        *) log "invalid version argument"; exit 1 ;;
    esac

    log "wrote files and diffs"
}

main "$@"
