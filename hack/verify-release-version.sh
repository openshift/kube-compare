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
    local output command_name version_word actual_version build_date extra

    case "$(basename "${binary}")" in
        kubectl-cluster_compare) expected_name=cluster-compare ;;
        helm-convert) expected_name=helm-convert ;;
        report-creator) expected_name=create-report ;;
        *) fail "Unexpected release binary: ${binary}" ;;
    esac

    output=$("${binary}" --version) || fail "Failed to read version from ${binary}."
    read -r command_name version_word actual_version build_date extra <<<"${output}"

    [[ "${command_name}" == "${expected_name}" &&
        "${version_word}" == "version" &&
        "${actual_version}" == "${expected}" &&
        "${build_date}" =~ ^\([0-9]{4}-(0[1-9]|1[0-2])-(0[1-9]|[12][0-9]|3[01])T([01][0-9]|2[0-3]):[0-5][0-9]:[0-5][0-9]Z\)$ &&
        -z "${extra:-}" ]] ||
        fail "Unexpected version output from $(basename "${binary}"): ${output}; expected ${expected_name} version ${expected} (RFC3339 date)."

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
