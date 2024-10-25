# Addon-tools

These tools were written as helpers to the main cluster-compare plugin, and are
not supported but are provided for convenience.

## helm-convert

This utility converts a cluster-compare metadata.yaml reference to a helm
chart, which can then be used to deploy the CRs or render the CRs locally.

## report-creator

This utilitiy consumes the output.json from cluster-compare and creates a
junit.xml that matches, for integration in pipelines that like junit.xml
