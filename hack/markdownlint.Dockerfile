# Following example of: https://github.com/openshift/enhancements/blob/master/hack/Dockerfile.markdownlint
FROM registry.access.redhat.com/ubi9/ubi:latest
WORKDIR /workdir
RUN dnf install -y git golang
COPY markdownlint-install.sh /tmp
RUN /tmp/markdownlint-install.sh
ENTRYPOINT /workdir/hack/markdownlint.sh
