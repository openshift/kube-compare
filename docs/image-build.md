# Kube-compare image build

This image is also distributed in a container build, mostly for Red Hat downstream distribution purposes.

## Building the container

```bash
make image-build
```

## Copy binary locally

- One option it to build the binary locally

```bash
make cross-build
```

## Extracting the tool from the container

```bash
ENGINE=podman # works with "docker" too
IMAGE=kube-compare:latest
BASEOS=rhel9 # "rhel8" is an option too, for older systems
$ENGINE create --name kube-compare "$IMAGE"
$ENGINE cp "kube-compare:/usr/share/openshift/linux_amd64/kube-compare.$BASEOS" ./kubectl-cluster_compare
$ENGINE rm -f kube-compare

# run 
export PATH=$PWD:$PATH
oc cluster-compare -h
```
