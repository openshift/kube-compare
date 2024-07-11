
# Kube-compare image build

## Build container

```bash
make image-build
```

## Copy binary locally

- One option it to build the binary locally

```bash
make cross-build
```

- Another option is to extract the binary from the container

```bash
docker create --name kube-compare kube-compare:latest
docker cp kube-compare:/usr/share/openshift/linux_amd64/kube-compare.rhel9 ./kube-compare.rhel9
docker rm -f kube-compare
```
