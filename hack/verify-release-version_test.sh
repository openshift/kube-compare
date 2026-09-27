#!/bin/bash

set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
verify_script="${repo_root}/hack/verify-release-version.sh"
tmp_dir=$(mktemp -d)
trap 'rm -rf "${tmp_dir}"' EXIT

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

expect_failure() {
    local name=$1 pattern=$2
    shift 2
    if "$@" >"${tmp_dir}/${name}.out" 2>"${tmp_dir}/${name}.err"; then
        fail "${name} unexpectedly succeeded"
    fi
    grep -Eq "${pattern}" "${tmp_dir}/${name}.err" || {
        cat "${tmp_dir}/${name}.err" >&2
        fail "${name} did not report ${pattern}"
    }
    echo "PASS: ${name} rejected"
}

write_binary() {
    local path=$1 output=$2
    mkdir -p "$(dirname "${path}")"
    printf '#!/bin/bash\nprintf '\''%%s\\n'\'' %q\n' "${output}" >"${path}"
    chmod +x "${path}"
}

make_archives() {
    local fixture=$1 main_output=$2 helm_output=$3 report_output=$4
    local contents="${fixture}/contents"
    rm -rf "${contents}" "${fixture}/dist"
    mkdir -p "${contents}/main" "${contents}/addons" "${fixture}/dist"
    write_binary "${contents}/main/kubectl-cluster_compare" "${main_output}"
    write_binary "${contents}/addons/helm-convert" "${helm_output}"
    write_binary "${contents}/addons/report-creator" "${report_output}"
    tar -czf "${fixture}/dist/kube-compare_linux_amd64.tar.gz" -C "${contents}/main" .
    tar -czf "${fixture}/dist/kube-compare_addon_tools_linux_amd64.tar.gz" -C "${contents}/addons" .
}

version=1.2.3
date=2026-09-27T01:35:39Z
valid_main="cluster-compare version ${version} (${date})"
valid_helm="helm-convert version ${version} (${date})"
valid_report="create-report version ${version} (${date})"
fixture="${tmp_dir}/fixture"
mkdir -p "${fixture}"

make_archives "${fixture}" "${valid_main}" "${valid_helm}" "${valid_report}"
EXPECTED_RELEASE_VERSION=${version} DIST_DIR="${fixture}/dist" "${verify_script}" >"${tmp_dir}/positive.out"
grep -Fx "kubectl-cluster_compare: ${valid_main}" "${tmp_dir}/positive.out"
grep -Fx "helm-convert: ${valid_helm}" "${tmp_dir}/positive.out"
grep -Fx "report-creator: ${valid_report}" "${tmp_dir}/positive.out"
echo "PASS: packaged binaries report the exact expected version"

declare -a invalid_outputs=(
    "cluster-compare version 1.2.2 (${date})"
    "cluster-compare version 11.2.3 (${date})"
    "cluster-compare version 1.2.3-rc1 (${date})"
    "cluster-compare version unreleased (${date})"
    "cluster-compare version ${version} (unknown)"
    "cluster-compare version ${version} (2026-09-27)"
    "cluster-compare version ${version} (2026-19-27T01:35:39Z)"
)

for index in "${!invalid_outputs[@]}"; do
    make_archives "${fixture}" "${invalid_outputs[${index}]}" "${valid_helm}" "${valid_report}"
    expect_failure "invalid-output-${index}" "Unexpected version output" \
        env EXPECTED_RELEASE_VERSION=${version} DIST_DIR="${fixture}/dist" "${verify_script}"
done

rm -f "${fixture}/dist/kube-compare_addon_tools_linux_amd64.tar.gz"
expect_failure missing-artifact "Expected release archive not found" \
    env EXPECTED_RELEASE_VERSION=${version} DIST_DIR="${fixture}/dist" "${verify_script}"

write_binary "${fixture}/kubectl-cluster_compare" "${valid_main}"
EXPECTED_RELEASE_VERSION=${version} "${verify_script}" --binary linux_amd64_v1 "${fixture}/kubectl-cluster_compare"
EXPECTED_RELEASE_VERSION=${version} "${verify_script}" --binary darwin_arm64_v8.0 "${fixture}/does-not-exist"

fake_path="${tmp_dir}/fake-path"
mkdir -p "${fake_path}"
printf '%s\n' '#!/bin/bash' '[[ "$1 $2" == "env GOHOSTOS" ]] && echo darwin || echo arm64' >"${fake_path}/go"
chmod +x "${fake_path}/go"
expect_failure unsupported-host "supported only on linux/amd64; detected darwin/arm64" \
    env PATH="${fake_path}:${PATH}" EXPECTED_RELEASE_VERSION=${version} "${verify_script}" --preflight

git_fixture="${tmp_dir}/git-fixture"
git init -q "${git_fixture}"
git -C "${git_fixture}" config user.email test@example.com
git -C "${git_fixture}" config user.name Test
touch "${git_fixture}/tracked"
git -C "${git_fixture}" add tracked
git -C "${git_fixture}" commit -qm initial
git -C "${git_fixture}" tag v1.2.3
(
    cd "${git_fixture}"
    RELEASE_REQUIRE_EXACT_TAG=1 "${verify_script}" --preflight
)

touch "${git_fixture}/dirty"
expect_failure dirty-tree "require a clean worktree" \
    bash -c "cd '${git_fixture}' && RELEASE_REQUIRE_EXACT_TAG=1 '${verify_script}' --preflight"
rm "${git_fixture}/dirty"
echo next >"${git_fixture}/tracked"
git -C "${git_fixture}" commit -qam next
expect_failure untagged-head "require an exact vMAJOR.MINOR.PATCH tag" \
    bash -c "cd '${git_fixture}' && RELEASE_REQUIRE_EXACT_TAG=1 '${verify_script}' --preflight"
git -C "${git_fixture}" tag release-1.2.4
expect_failure malformed-tag "must match vMAJOR.MINOR.PATCH" \
    bash -c "cd '${git_fixture}' && RELEASE_REQUIRE_EXACT_TAG=1 '${verify_script}' --preflight"

echo "All release version verification tests passed."
