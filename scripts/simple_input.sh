#!/usr/bin/env bash
# shellcheck disable=SC2155

set -euo pipefail

log() {
    echo "$(date +"%H:%M:%S") - $(printf '%s' "$@")" 1>&2
}

readonly ROOT_DIR="$(realpath "$(dirname "$0")/..")"
readonly INPUT_DIR="${ROOT_DIR}/input"

v1() {
    local dir="${1}"

    mkdir -p "${dir}"
    echo "a" > "${dir}/a"
    echo "b" > "${dir}/b"

    log "wrote v1"
}

v2() {
    local v1="${1}"
    local dir="${2}"

    cp -r "${v1}" "${dir}"
    echo "c" > "${dir}/c"

    log "wrote v2"
}

v3() {
    local v2="${1}"
    local dir="${2}"

    cp -r "${v2}" "${dir}"
    echo "d" > "${dir}"/a
    echo "e" > "${dir}"/b

    log "wrote v3"
}

list_all_files() {
    local dir="${1#$ROOT_DIR/}"
    local output="${2}"

    (
        cd "${ROOT_DIR}"
        find "${dir}" -type f > "${output}"
    )
}

list_diff_files() {
    local before="${1#$ROOT_DIR/}"
    local after="${2#$ROOT_DIR/}"
    local output="${3}"

    (
        cd "${ROOT_DIR}"
        git diff --name-only --no-index --diff-filter=d -l0 "${before}" "${after}" > "${output}" || true
    )
}

main() {
    log "writing simple inputs to ${INPUT_DIR}"
    rm -rf "${INPUT_DIR:?}"
    mkdir -p "${INPUT_DIR}"

    local v1_dir="${INPUT_DIR}/v1"
    local v2_dir="${INPUT_DIR}/v2"
    local v3_dir="${INPUT_DIR}/v3"

    v1 "${v1_dir}"
    v2 "${v1_dir}" "${v2_dir}"
    v3 "${v2_dir}" "${v3_dir}"

    list_all_files "${v1_dir}" "${INPUT_DIR}/initial.txt"
    list_diff_files "${v1_dir}" "${v2_dir}" "${INPUT_DIR}/diff_v1_v2.txt"
    list_diff_files "${v2_dir}" "${v3_dir}" "${INPUT_DIR}/diff_v2_v3.txt"

    log "wrote file and diff lists"
}

main "$@"
