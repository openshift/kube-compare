#!/bin/bash -e

TAG=$(gh release view --json name | jq -r .name)
checksums=$(gh release download --pattern '*_checksums.txt' -O -)
echo "Creating krew bump for $TAG"

# Tmpdir for krew-index clone
mkdir -p _release_tmp
pushd _release_tmp
gh repo fork --clone kubernetes-sigs/krew-index
pushd krew-index
BRANCH="cluster-compare_$TAG"
git checkout -b "${BRANCH}" upstream/master

# Update version number
echo "  Updating version to $TAG"
VERSION="$TAG" yq -i '.spec.version = strenv(VERSION)' plugins/cluster-compare.yaml

# Update artifact hashes
while read -r checksum filename; do
    uri="https://github.com/openshift/kube-compare/releases/download/$TAG/$filename"
    echo "  Updating artifact $uri ($checksum)"
    FILENAME="$filename" URI="$uri" SHA="$checksum" yq -i '(.spec.platforms[] | select(.uri|contains(strenv(FILENAME)))) |= . + {"uri": strenv(URI), "sha256": strenv(SHA)}' plugins/cluster-compare.yaml
done <<<"$checksums"

# Commit and push
git commit -asm "Version bump cluster-compare to $TAG"
git push -u origin "$BRANCH"
gh pr create --fill

# Cleanup
popd && popd && rm -rf _release_tmp
