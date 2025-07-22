# Baseline reference configuration

This repo provides a simple example reference configuration for the `cluster-compare` plugin.

## Goal

Create a minimal baseline reference configuration that works with `oc cluster-compare` on any OpenShift cluster. This helps users get started with the cluster-compare plugin quickly and easily.

## Usage

Run the reference configuration against your cluster:

```bash
oc cluster-compare -r https://raw.githubusercontent.com/openshift/kube-compare/main/docs/example/metadata.yaml
```
