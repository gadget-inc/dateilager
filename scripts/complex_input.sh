#!/usr/bin/env bash
# shellcheck disable=SC2155

set -euo pipefail

log() {
    echo "$(date +"%H:%M:%S") - $(printf '%s' "$@")" 1>&2
}

readonly ROOT_DIR="$(realpath "$(dirname "$0")/..")"
readonly INPUT_DIR="${ROOT_DIR}/input"

build_node_modules() {
    local package_json="${1}"
    local output="${2}"
    local tmpdir="$(mktemp -d -t dl-XXXXXXXXXX)"

    cp "${package_json}" "${tmpdir}/package.json"
    (
        cd "${tmpdir}"
        npm install &> /dev/null
        cp -r ./node_modules/* "${output}"
    )

    rm -rf "${tmpdir:?}"
}

v1() {
    local dir="${1}"

    mkdir -p "${dir}"
    build_node_modules "${ROOT_DIR}/scripts/package-v1.json" "${dir}"

    log "wrote v1 to ${dir}"
}

v2() {
    local dir="${1}"

    mkdir -p "${dir}"
    build_node_modules "${ROOT_DIR}/scripts/package-v2.json" "${dir}"

    log "wrote v2 to ${dir}"
}

v3() {
    local dir="${1}"

    mkdir -p "${dir}"
    build_node_modules "${ROOT_DIR}/scripts/package-v3.json" "${dir}"

    log "wrote v3 to ${dir}"
}

generate_diff() {
    local dir="${1}"
    local sum="${2}"
    local out="${3}"

    fsdiff -dir "${dir}" -sum "${sum}" -out "${out}"
}

main() {
    log "writing complex inputs to ${INPUT_DIR}"
    rm -rf "${INPUT_DIR:?}"/*
    mkdir -p "${INPUT_DIR}"

    local v1_dir="${INPUT_DIR}/v1"
    local v2_dir="${INPUT_DIR}/v2"
    local v3_dir="${INPUT_DIR}/v3"

    v1 "${v1_dir}"
    v2 "${v2_dir}"
    v3 "${v3_dir}"

    generate_diff "${v1_dir}" "" "${v1_dir}_state"
    generate_diff "${v2_dir}" "${v1_dir}_state/sum.zst" "${v2_dir}_state"
    generate_diff "${v3_dir}" "${v2_dir}_state/sum.zst" "${v3_dir}_state"

    log "wrote files and diffs"
}

main "$@"
