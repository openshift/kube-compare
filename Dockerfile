# This Dockerfile builds an image containing the Mac and Windows version of kube-compare
# layered on top of the Linux cli image.
FROM registry.ci.openshift.org/ocp/builder:rhel-8-golang-1.22-builder-multi-openshift-4.17 AS builder-rhel-8
WORKDIR /go/src/github.com/openshift/kube-compare
COPY . .
RUN make cross-build --warn-undefined-variables

FROM registry.ci.openshift.org/ocp/builder:rhel-9-golang-1.22-builder-multi-openshift-4.17 AS builder-rhel-9
WORKDIR /go/src/github.com/openshift/kube-compare
COPY . .
RUN make cross-build --warn-undefined-variables

FROM --platform=linux/amd64 registry.ci.openshift.org/ocp/4.17:cli as final-builder

COPY --from=builder-rhel-9 /go/src/github.com/openshift/kube-compare/_output/bin/ /usr/share/openshift/

RUN cd /usr/share/openshift && \
    ln -sf /usr/share/openshift/linux_amd64/kube-compare /usr/bin/kube-compare && \
    mv windows_amd64 windows && \
    mv darwin_amd64 mac && \
    mv darwin_arm64 mac_arm64

COPY --from=builder-rhel-8 /go/src/github.com/openshift/kube-compare/_output/bin/linux_amd64/kubectl-cluster_compare /usr/share/openshift/linux_amd64/kube-compare.rhel8
COPY --from=builder-rhel-8 /go/src/github.com/openshift/kube-compare/_output/bin/linux_arm64/kubectl-cluster_compare /usr/share/openshift/linux_arm64/kube-compare.rhel8
COPY --from=builder-rhel-8 /go/src/github.com/openshift/kube-compare/_output/bin/linux_ppc64le/kubectl-cluster_compare /usr/share/openshift/linux_ppc64le/kube-compare.rhel8
COPY --from=builder-rhel-8 /go/src/github.com/openshift/kube-compare/_output/bin/linux_s390x/kubectl-cluster_compare /usr/share/openshift/linux_s390x/kube-compare.rhel8

COPY --from=builder-rhel-9 /go/src/github.com/openshift/kube-compare/_output/bin/linux_amd64/kubectl-cluster_compare /usr/share/openshift/linux_amd64/kube-compare.rhel9
COPY --from=builder-rhel-9 /go/src/github.com/openshift/kube-compare/_output/bin/linux_arm64/kubectl-cluster_compare /usr/share/openshift/linux_arm64/kube-compare.rhel9
COPY --from=builder-rhel-9 /go/src/github.com/openshift/kube-compare/_output/bin/linux_ppc64le/kubectl-cluster_compare /usr/share/openshift/linux_ppc64le/kube-compare.rhel9
COPY --from=builder-rhel-9 /go/src/github.com/openshift/kube-compare/_output/bin/linux_s390x/kubectl-cluster_compare /usr/share/openshift/linux_s390x/kube-compare.rhel9

COPY --from=builder-rhel-8 /go/src/github.com/openshift/kube-compare/LICENSE /usr/share/openshift/LICENSE

WORKDIR /usr/share/openshift/
