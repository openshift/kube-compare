#!/bin/bash -eu

DIST_DIR=${DIST_DIR:-dist}
host_os=$(go env GOHOSTOS)
host_arch=$(go env GOHOSTARCH)

if [[ "${host_os}/${host_arch}" != "linux/amd64" ]]; then
    echo "Release version verification is supported only on linux/amd64; detected ${host_os}/${host_arch}." >&2
    exit 1
fi

if [[ "${1:-}" == "--preflight" ]]; then
    echo "Release version verification supports host ${host_os}/${host_arch}."
    exit 0
fi

if [[ $# -ne 0 ]]; then
    echo "Usage: $0 [--preflight]" >&2
    exit 1
fi

main_archive="${DIST_DIR}/kube-compare_linux_amd64.tar.gz"
addon_archive="${DIST_DIR}/kube-compare_addon_tools_linux_amd64.tar.gz"

for archive in "${main_archive}" "${addon_archive}"; do
    if [[ ! -f "${archive}" ]]; then
        echo "Expected release archive not found: ${archive}" >&2
        exit 1
    fi
done

tmp_dir=$(mktemp -d)
trap 'rm -rf "${tmp_dir}"' EXIT

tar -xzf "${main_archive}" -C "${tmp_dir}"
tar -xzf "${addon_archive}" -C "${tmp_dir}"

for binary in kubectl-cluster_compare helm-convert report-creator; do
    version_output=$("${tmp_dir}/${binary}" --version)
    echo "${binary}: ${version_output}"

    if grep -Eiq 'unreleased|unknown' <<<"${version_output}"; then
        echo "Release metadata was not injected into ${binary}" >&2
        exit 1
    fi
done
