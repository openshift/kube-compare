# Red Hat downstream build notes

This build is managed by ART, so most changes need to go through them.

## Contact info

- ART slack room: [#forum-ocp-art](https://redhat.enterprise.slack.com/archives/CB95J6R4N)

## Build information

- [art build status](https://art-build-history-art-build-history.apps.artc2023.pc3z.p1.openshiftapps.com/?name=kube-compare-art)
  - Staging pullspec: `registry.stage.redhat.io/openshift4/kube-compare-artifacts-rhel9:v4.21`
- [Errata](https://errata.engineering.redhat.com/package/show/kube-compare-artifacts-container)
- [Downstream container](https://catalog.redhat.com/software/containers/openshift4/kube-compare-artifacts-rhel9/66d56abedf3259c57cfc8cba)
  - Release pullspec: registry.redhat.io/openshift4/kube-compare-artifacts-rhel9:v4.19

## Installing the downstream version of the tool

```bash
TAG=v4.19 # Check the downstream container page for the highest released version tag (do not use `latest`)
ENGINE=podman # works with "docker" too
IMAGE=registry.redhat.io/openshift4/kube-compare-artifacts-rhel9:$TAG
BASEOS=rhel9 # "rhel8" is an option too, for older systems
$ENGINE create --name kube-compare "$IMAGE"
$ENGINE cp "kube-compare:/usr/share/openshift/linux_amd64/kube-compare.$BASEOS" ./kubectl-cluster_compare
$ENGINE rm -f kube-compare
sudo install kubectl-cluster_compare /usr/local/bin
rm kubectl-cluster_compare

# run
oc cluster-compare -h
```
