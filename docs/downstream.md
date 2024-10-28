# Red Hat downstream build notes

This build is managed by ART, so most changes need to go through them.

## Contact info

- ART slack room: [#forum-ocp-art](https://redhat.enterprise.slack.com/archives/CB95J6R4N)

## Build information

- [art-dash build status](https://art-dash.engineering.redhat.com/dashboard/build/history?dg_name=kube-compare-artifacts)
- [brew package](https://brewweb.engineering.redhat.com/brew/packageinfo?packageID=86274)
- [Errata](https://errata.engineering.redhat.com/package/show/kube-compare-artifacts-container)
- [Downstream container](https://catalog.redhat.com/software/containers/openshift4/kube-compare-artifacts-rhel9/66d56abedf3259c57cfc8cba)
  - Pullspec: registry.redhat.io/openshift4/kube-compare-artifacts-rhel9:latest

## Installing the downstream version of the tool

```bash
ENGINE=podman # works with "docker" too
IMAGE=registry.redhat.io/openshift4/kube-compare-artifacts-rhel9:latest
BASEOS=rhel9 # "rhel8" is an option too, for older systems
$ENGINE create --name kube-compare "$IMAGE"
$ENGINE cp "kube-compare:/usr/share/openshift/linux_amd64/kube-compare.$BASEOS" ./kubectl-cluster_compare
$ENGINE rm -f kube-compare
sudo install kubectl-cluster_compare /usr/local/bin
rm kubectl-cluster_compare

# run
oc cluster-compare -h
```
