#!/bin/bash

set -euo pipefail

DIST_DIR=${DIST_DIR:-dist}

usage() {
    cat >&2 <<EOF
Usage:
  EXPECTED_RELEASE_VERSION=X.Y.Z $0 --preflight
  RELEASE_REQUIRE_EXACT_TAG=1 $0 --preflight
  $0 --binary <goreleaser-target> <binary-path>
  EXPECTED_RELEASE_VERSION=X.Y.Z $0

EXPECTED_RELEASE_VERSION is intended for non-publishing dry runs and tests.
Real releases must set RELEASE_REQUIRE_EXACT_TAG=1; their expected version is
derived from the exact vMAJOR.MINOR.PATCH tag at HEAD.
EOF
}

fail() {
    echo "$*" >&2
    exit 1
}

require_supported_host() {
    local host_os host_arch
    host_os=$(go env GOHOSTOS)
    host_arch=$(go env GOHOSTARCH)

    if [[ "${host_os}/${host_arch}" != "linux/amd64" ]]; then
        fail "Release version verification is supported only on linux/amd64; detected ${host_os}/${host_arch}."
    fi

    echo "Release version verification supports host ${host_os}/${host_arch}."
}

expected_version() {
    local tag version status

    if [[ "${RELEASE_REQUIRE_EXACT_TAG:-}" == "1" ]]; then
        status=$(git status --porcelain)
        [[ -z "${status}" ]] || fail "Real releases require a clean worktree."

        if ! tag=$(git describe --tags --exact-match HEAD 2>/dev/null); then
            fail "Real releases require an exact vMAJOR.MINOR.PATCH tag at HEAD."
        fi
        [[ "${tag}" =~ ^v([0-9]+\.[0-9]+\.[0-9]+)$ ]] ||
            fail "Real release tag must match vMAJOR.MINOR.PATCH; found ${tag}."
        version=${BASH_REMATCH[1]}
    else
        version=${EXPECTED_RELEASE_VERSION:-}
        [[ -n "${version}" ]] || {
            usage
            fail "EXPECTED_RELEASE_VERSION is required for dry runs and tests."
        }
        [[ "${version}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] ||
            fail "Expected release version must match MAJOR.MINOR.PATCH; found ${version}."
    fi

    echo "${version}"
}

verify_binary() {
    local binary=$1 expected=$2 expected_name
    local output_file output_fd output extra output_debug
    local output_pattern command_name actual_version build_date canonical_output
    local date_probe normalized_date

    case "$(basename "${binary}")" in
        kubectl-cluster_compare) expected_name=cluster-compare ;;
        helm-convert) expected_name=helm-convert ;;
        report-creator) expected_name=create-report ;;
        *) fail "Unexpected release binary: ${binary}" ;;
    esac

    output_file=$(mktemp) || fail "Failed to create temporary file for version output."
    if ! "${binary}" --version >"${output_file}"; then
        rm -f "${output_file}"
        fail "Failed to read version from ${binary}."
    fi

    # Read stdout from a file so command substitution cannot discard trailing
    # newlines. A canonical response is exactly one newline-terminated line.
    output=
    extra=
    exec {output_fd}<"${output_file}"
    if ! IFS= read -r output <&"${output_fd}" ||
        IFS= read -r extra <&"${output_fd}" ||
        [[ -n "${extra}" ]]; then
        exec {output_fd}<&-
        printf -v output_debug '%q' "$(<"${output_file}")"
        rm -f "${output_file}"
        fail "Unexpected version output from $(basename "${binary}"): ${output_debug}; expected exactly one canonical newline-terminated output line."
    fi
    exec {output_fd}<&-

    output_pattern='^([^[:space:]]+) version ([^[:space:]]+) \(([0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z)\)$'
    if [[ "${output}" =~ ${output_pattern} ]]; then
        command_name=${BASH_REMATCH[1]}
        actual_version=${BASH_REMATCH[2]}
        build_date=${BASH_REMATCH[3]}
    else
        rm -f "${output_file}"
        fail "Unexpected version output from $(basename "${binary}"): ${output}; expected ${expected_name} version ${expected} (RFC3339 UTC date)."
    fi

    canonical_output="${expected_name} version ${expected} (${build_date})"
    if [[ "${command_name}" != "${expected_name}" ||
        "${actual_version}" != "${expected}" ||
        "${output}" != "${canonical_output}" ]] ||
        ! printf '%s\n' "${canonical_output}" | cmp -s - "${output_file}"; then
        rm -f "${output_file}"
        fail "Unexpected version output from $(basename "${binary}"): ${output}; expected ${expected_name} version ${expected} (RFC3339 UTC date)."
    fi
    rm -f "${output_file}"

    command -v date >/dev/null 2>&1 ||
        fail "Cannot validate build date for $(basename "${binary}"): date utility is unavailable."
    if ! date_probe=$(LC_ALL=C date --utc --date='2000-02-29T00:00:00Z' '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null) ||
        [[ "${date_probe}" != "2000-02-29T00:00:00Z" ]]; then
        fail "Cannot validate build date for $(basename "${binary}"): a GNU-compatible date utility is required."
    fi
    if ! normalized_date=$(LC_ALL=C date --utc --date="${build_date}" '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null) ||
        [[ "${normalized_date}" != "${build_date}" ]]; then
        fail "Unexpected version output from $(basename "${binary}"): build date ${build_date} is not a real RFC3339 UTC instant."
    fi

    echo "$(basename "${binary}"): ${output}"
}

case "${1:-}" in
    --preflight)
        [[ $# -eq 1 ]] || {
            usage
            exit 1
        }
        require_supported_host
        version=$(expected_version)
        echo "Expected release version: ${version}"
        ;;
    --binary)
        [[ $# -eq 3 ]] || {
            usage
            exit 1
        }
        target=$2
        binary=$3
        case "${target}" in
            linux_amd64*) ;;
            *)
                echo "Skipping release version verification for target ${target}."
                exit 0
                ;;
        esac
        version=$(expected_version)
        verify_binary "${binary}" "${version}"
        ;;
    "")
        version=$(expected_version)
        main_archive="${DIST_DIR}/kube-compare_linux_amd64.tar.gz"
        addon_archive="${DIST_DIR}/kube-compare_addon_tools_linux_amd64.tar.gz"

        for archive in "${main_archive}" "${addon_archive}"; do
            [[ -f "${archive}" ]] || fail "Expected release archive not found: ${archive}"
        done

        tmp_dir=$(mktemp -d)
        trap 'rm -rf "${tmp_dir}"' EXIT
        tar -xzf "${main_archive}" -C "${tmp_dir}"
        tar -xzf "${addon_archive}" -C "${tmp_dir}"

        for binary in kubectl-cluster_compare helm-convert report-creator; do
            verify_binary "${tmp_dir}/${binary}" "${version}"
        done
        ;;
    *)
        usage
        exit 1
        ;;
esac
