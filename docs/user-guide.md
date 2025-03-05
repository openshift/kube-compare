# User Guide

## Rationale

The “kubectl cluster-compare” command is capable of performing an intelligent diff between a reference configuration and
the specific configuration applied to a cluster. The comparison is capable of suppressing diffs of content which is
expected to be user variable, validating required and optional configuration, and ignoring known runtime variable
fields. With these fields suppressed the user is able to focus on the remaining diffs which are more
relevant/potentially impactful to the use case. With these capabilities a cluster administrator, solutions architect,
support engineers, and others can validate a cluster’s configuration against a baseline reference configuration.

## Concepts

### Reference Configuration

The target of `kubectl cluster compare` tool is to detect deviations from a live or local set of CRs with respect to a Reference Configuration specified by a set of recommended Custom Resources (CRs).

But first, what do we understand by Reference Configuration? The Reference Configuration consists of a set of CRs and an entry-point which defines the expected mandatory and optional components. This entrypoint is a YAML file called `metadata.yaml`.

- A Reference Configuration is typically versioned to account for necessary differences in the configuration based on the specific cluster version.
- The top level (entry point) of the reference configuration is the metadata descriptor YAML file (`metadata.yaml`). This file defines the Reference Configuration.
- A published `metadata.yaml` identifies the set of compliant Reference Configuration CR objects for the covered use case. Any differences in cluster configuration found when running the `kubectl cluster compare` tool shall be identified as a deviation from the expected Reference Configuration.

For guidance on the Reference Configuration defined by `metadata.yaml` see
[this guide](./reference-config-guide.md).

### Optional vs required

### CR selection (correlation)

The "kubectl cluster-compare" tool works by iterating across the CRs found in the reference configuration and finding
CRs from the users configuration (live cluster or collection of CRs) which should be compared to the
reference. Typically the relevant user CRs have unique names or are contained in user defined namespaces. The tool
correlates reference CRs to user CRs as described in this section in order to account for the expected variations in
naming without requiring the user to explicitly map reference to cluster CR.

#### Correlating CRs

`kubectl cluster-compare` must correlate CRs between reference and input configurations to perform the
comparisons. `kubectl cluster-compare` correlates CRs by using the apiVersion, kind, namespace and name fields of the
CRs to perform a nearest match correlation. Optionally the user may provide a manual override of the correlation to
identify a specific reference configuration CR to be used for a given user input CR. Manual matches are prioritized over
the automatic of correlation, meaning manual matches override matches by similar values in the specified group of
fields.

##### Correlation by manual matches

`kubectl cluster-compare` gets as input a diff config that contains an option to specify manual matches between cluster
resources and resource templates. The matches can be added to the config as pairs of `apiVersion_kind_namespace_name:
<Template File Name>`. For cluster scoped CRs that don't have a namespace the matches can be added as pairs of
`apiVersion_kind_name: <Template File Name>`.

##### Correlation by group of fields (apiVersion, kind, namespace and name)

When there is no manual match for a CR the command will try to match a template for the resource by looking at the
4-tuple: apiVersion, kind, namespace and name . The Correlation is based on which fields in the templates that are not
user-variable. Templates get matched to resources based on all the features from the 4-tuple that are declared fixed (
not user-variable) in the templates.

For example a template with a fixed namespace, kind, name and templated (user-variable) apiVersion will only be a
potential match by the kind-namespace-name criterion.

For each resource the group correlation will be done by the next logic:

1. Exact match of apiVersion-kind-namespace-name
    1. If there is a result in the reference, comparison will be done
1. Exact Match in 3/4 fields from apiVersion, kind, namespace, name. ( meaning exact match in: kind-namespace-name or
   apiVersion-kind-name or apiVersion-kind-namespace)
    1. If there is a result in the reference, comparison will be done
1. Exact Match in 2/4 fields from apiVersion, kind, namespace, name. ( meaning exact match in: kind-namespace or
   kind-name or apiVersion-kind)
    1. If there is a result in the reference, comparison will be done
1. Match kind
    1. If there is a result in the reference, comparison will be done
1. No match – comparison cannot be made and the file is flagged as unmatched.

We can phrase this logic in a more general form. Each CR will be correlated to a template with an exact match in the
largest number of fields from this group:  apiVersion, kind, namespace, name.

### How it works

- eg how templates pull content into reference prior to compare

### Limits

### Single CR scope

This tool provides a context aware diff function. In some cases the reference may further provide validation of values
in the CRs. This validation operates within the scope of a the current CR. Any validation of values across multiple CRs
is out of scope.

### CR validation

This tool performs a diff of a CR against the reference. It does not have access to resources outside the scope of what
is available through the cluster's API. This places validation of the configuration against underlying platform
hardware, os configuration, etc (unless available through the api) out of scope.

## Example use cases

To Compare a known valid reference configuration with a live cluster:

`kubectl cluster-compare -r <referenceConfigurationDirectory>`

To Compare a known valid reference configuration with a local set of CRs:

`kubectl cluster-compare -r <referenceConfigurationDirectory> -f <inputConfiguration>`

To Compare a known valid reference configuration with a live cluster and with a user config:

`kubectl cluster-compare -r <referenceConfigurationDirectory> -c <userConfig>`

To Run a known valid reference configuration with a support archive output:

`kubectl cluster-compare -r <referenceConfigurationDirectory> -f "must-gather*/*/cluster-scoped-resources","must-gather*/*/namespaces" -R`

## Understanding the output

### States of a Reference Configuration CR after running the tool

After running `kubectl cluster-compare` tool, the reference configuration CRs will be identified in one of the following states: missing and present in the live cluster.

1. Missing CR. A `missing` reference CR can be in the following state:
    1. Required: Reference CR is required and does not appear in the target cluster CRs. Note that a missed reference CRs declared as optional is not going to be reported as missed.

2. Present CR. A `present` reference CR can be in one of the following states:
    1. Matched once: The reference CR has one correlated instance in the live cluster.
    2. Matched more than once: The reference CR has more than one correlated instance in the live cluster. There are additional reference CRs in the live cluster with equivalent apiVersion-kind-namespace-name.
    3. Present and unmatched: The reference configuration CR is present, which means that there is a match for api-kind-name-namespace, in the target cluster but does not follow some configuration value specific to the live cluster. This should be identified as a deviation.

## Options and advanced usage

### Diff config

The command includes an option to pass a config that includes user specified settings regarding on how to do the diff.
The config is formatted as a YAML file.

#### Manual Correlation

There is an option to select for specific CRs the template that it will be diffed with, this will overwrite the default
correlating.
The matches between the cluster CRs and the templates can be added to the diff config as pairs of `apiVersion_kind_namespace_name:
<Template File Name>`. For cluster scoped CRs that don't have a namespace the matches can be added as pairs of
`apiVersion_kind_name: <Template File Name>`.
For example:

```yaml
correlationSettings:
   manualCorrelation:
      correlationPairs:
         apps.v1.DaemonSet.kube-system.kindnet.yaml: "template_example.yaml"
```

### Kubectl Environment Variables

The tool is responsive to KUBECTL_EXTERNAL_DIFF environment variable (same as kubectl diff). This allows you to tailor the output formatting to suit your preference.

-y : side by side comparison
--color: colored output
-W <char_width>: width of char_length

For more available options do diff --help

side-by-side comparison (total width 150 characters) with:
`KUBECTL_EXTERNAL_DIFF="diff -y -W 150"`

### Alternative output formats

## JSON

Adding `-o json` to the command line produces json output that can be more
easily parsed by other software.

For example:

```json
{
  "Summary": {
    "ValidationIssuses": {
      "ExamplePart": {
        "Dashboard": {
          "Msg": "Missing CRs",
          "CRs": [
            "deploymentDashboard.yaml"
          ]
        }
      }
    },
    "NumMissing": 1,
    "UnmatchedCRS": [],
    "NumDiffCRs": 1,
    "TotalCRs": 1,
    "MetadataHash": "aa4c94f1307788e1da81f57718a9f1364d35d4ff6099fc633724bcf9d051a094",
    "patchedCRs": 0
  },
  "Diffs": [
    {
      "DiffOutput": "diff -u -N TEMP/apps-v1_deployment_kubernetes-dashboard_dashboard-metrics-scraper TEMP/apps-v1_deployment_kubernetes-dashboard_dashboard-metrics-scraper\n--- TEMP/apps-v1_deployment_kubernetes-dashboard_dashboard-metrics-scraper\tDATE\n+++ TEMP/apps-v1_deployment_kubernetes-dashboard_dashboard-metrics-scraper\tDATE\n@@ -10,7 +10,7 @@\n   revisionHistoryLimit: 10\n   selector:\n     matchLabels:\n-      k8s-app: dashboard-metrics-scraper\n+      k8s-app: dashboard-metrics-scraper-diff\n   template:\n     metadata:\n       labels:\n",
      "CorrelatedTemplate": "deploymentMetrics.yaml",
      "CRName": "apps/v1_Deployment_kubernetes-dashboard_dashboard-metrics-scraper"
    }
  ]
}
```

## Junit.xml

Adding `-o junit` to the command line produces junit.xml output for easier
integration into CI pipelines.

The output consists of 3 testsuites:

- "Detected Differences" which shows every compared CR as a separate test case.
  These will be "pass" if there were no differences, "failed" if differences
  were detected, or "skipped" if differences were patched by a user override.
- "Reference validation" which shows every missing CR or violation of
  metadata.yaml cardinality rules (`OneOf`, `None`, `allOf`) with each
  problematic group and part of the metadata appearing as a separate testcase.
- "Unmatched Cluster Resources" which shows any cluster resources that could
  not be correlated with template CRs.

For each section, if no testcases are produced by the cluster-compare plugin, a
single passing testcase will be added.

## Troubleshooting

### False Positives

#### Reference Designs CRs may change (or not) from one cluster version to another version

The tool sometimes reports CR to be missing completely while in the live cluster environment the CR is present. This can happen at times because the tool is not correctly identifying the operator versions of these CRs or template's critical fields don't match cluster CRs.

##### Example #1

```diff
Missing required CRs:
Template-Reference:
  ExamplePart:
  - Example-Component-CR.yaml
  - Example-Component-CR.yaml

[cluster]$ kubectl get example-component -A
NAME                      AGE
example-component-name   5d20h
[cluster]$ kubectl get console -A
NAME                      AGE
example-component-name   5d20h
```

##### Example #2

```yaml
Cluster CR
apiVersion: ExampleVersion
kind: ExampleKind
metadata:
  name: $x-name
  namespace: ExampleNamespace

Template CR
apiVersion: ExampleVersion
kind: ExampleKind
metadata:
  name: $y-name
  namespace: ExampleNamespace
```

In such scenarios take these steps:

1. Ensure you are using the latest version of tool.

1. Ensure you are using the most up-to-date version of the reference configuration.

1. Ensure that template has the same api-version-kind-name-namespace as the cluster CR.

#### There can be more than one Reference Design CR of the same Kind

In this case you will have a warning presented before the diff output,
formatted similar to this: `The reference contains overlapping object
definitions which may result in unexpected correlation results. Re-run with
'--verbose' output enabled to view a detailed description of the issues`

This means the template contains two CRs with the same
apiversion-kind-name-namespace but different spec. In such cases for each
cluster CR the template that contains the least diffs will be used, to choose a
different template for the CR pass a user config (-c) and specify in the user
config file the template that should be matched to the CR. For info about the
exact syntax view the user config section.  As the warning text suggests,
running again with `--verbose` will list all the name collisions and which
templates are contributing to the issue.

This may also be intentional, in the case of 'OneOf' metadata directives, but
it is best practice to either have hard-coded names, or leave the names
templated so any CRs can match, if possible.

## Patching the reference

Reference templates have to cope with a lot of real world complexity, sometimes it isn't possible to encode all valid configurations.
There are sometimes also know exceptions to a reference.
These are encoded as patches applied to the refendered template output.

### Generating patches

We can generate the patches for you, via the following command:

```shell
kubectl cluster-compare -r <referenceConfigurationDirectory> -o 'generate-patches' --override-reason "A valid reason for the override" --generate-override-for "<Template path 1>" --generate-override-for "<Template path 2>"
```

This will write a file of patches for the templates `"<Template path 1>"` and `"<Template path 2>"`.
The reason will be displaced in the summary and should discribe the reason for patching that diff.

### Loading patches

Once you have a patch file you can then pass them into the normal command via the `-p/--overrides` flag as follows

```shell
kubectl cluster-compare -r <referenceConfigurationDirectory> -p <path to my patches file>
```

Any patches that are corrilated with resources will then be applied and diffs will be marked as patched and the patch reason supplied with be displated.

### Writting your own

Patches have three possible types `mergepatch`, `rfc6902` and `go-template` this is the same patch shown in all three types:

```yaml
---
- apiVersion: v1
  kind: Namespace
  name: openshift-storage
  reason: For the test
  templatePath: namespace.yaml
  type: mergepatch
  patch: '{"metadata":{"annotations":{"openshift.io/sa.scc.mcs":"s0:c29,c14","openshift.io/sa.scc.supplemental-groups":"1000840000/10000","openshift.io/sa.scc.uid-range":"1000840000/10000","reclaimspace.csiaddons.openshift.io/schedule":"@weekly","workload.openshift.io/allowed":null},"labels":{"kubernetes.io/metadata.name":"openshift-storage","olm.operatorgroup.uid/ffcf3f2d-3e37-4772-97bc-983cdfce128b":"","openshift.io/cluster-monitoring":"false","pod-security.kubernetes.io/audit":"privileged","pod-security.kubernetes.io/audit-version":"v1.24","pod-security.kubernetes.io/warn":"privileged","pod-security.kubernetes.io/warn-version":"v1.24","security.openshift.io/scc.podSecurityLabelSync":"true"}},"spec":{"finalizers":["kubernetes"]}}'
- name: openshift-storage
  apiVersion: v1
  kind: Namespace
  templatePath: namespace.yaml
  type: rfc6902
  reason: known deviation
  patch: '[
    {"op": "add", "path": "/metadata/annotations/openshift.io~1sa.scc.mcs", "value": "s0:c29,c14"}, 
    {"op": "add", "path": "/metadata/annotations/openshift.io~1sa.scc.supplemental-groups", "value": "1000840000/10000"}, 
    {"op": "add", "path": "/metadata/annotations/openshift.io~1sa.scc.uid-range", "value": "1000840000/10000"}, 
    {"op": "add", "path": "/metadata/annotations/reclaimspace.csiaddons.openshift.io~1schedule", "value": "@weekly"}, 
    {"op": "remove", "path": "/metadata/annotations/workload.openshift.io~1allowed"},
    {"op": "add", "path": "/metadata/labels/kubernetes.io~1metadata.name", "value": "openshift-storage"}, 
    {"op": "add", "path": "/metadata/labels/olm.operatorgroup.uid~1ffcf3f2d-3e37-4772-97bc-983cdfce128b", "value": ""}, 
    {"op": "add", "path": "/metadata/labels/openshift.io~1cluster-monitoring", "value": "false"}, 
    {"op": "add", "path": "/metadata/labels/pod-security.kubernetes.io~1audit", "value": "privileged"}, 
    {"op": "add", "path": "/metadata/labels/pod-security.kubernetes.io~1audit-version", "value": "v1.24"}, 
    {"op": "add", "path": "/metadata/labels/pod-security.kubernetes.io~1warn", "value": "privileged"}, 
    {"op": "add", "path": "/metadata/labels/pod-security.kubernetes.io~1warn-version", "value": "v1.24"}, 
    {"op": "add", "path": "/metadata/labels/security.openshift.io~1scc.podSecurityLabelSync", "value": "true"}, 
    {"op": "add", "path": "/spec", "value": {"finalizers": ["kubernetes"]}}
    ]'
- apiVersion: v1
  kind: Namespace
  name: openshift-storage
  reason: "known deviation"
  templatePath: namespace.yaml
  type: go-template
  patch: |
    {
        "type": "rfc6902",
        "patch": '[
            {"op": "add", "path": "/metadata/annotations/openshift.io~1sa.scc.mcs", "value": "s0:c29,c14"},
            {"op": "add", "path": "/metadata/annotations/openshift.io~1sa.scc.supplemental-groups", "value": "1000840000/10000"},
            {"op": "add", "path": "/metadata/annotations/openshift.io~1sa.scc.uid-range", "value": "1000840000/10000"},
            {"op": "add", "path": "/metadata/annotations/reclaimspace.csiaddons.openshift.io~1schedule", "value": "@weekly"},
            {"op": "remove", "path": "/metadata/annotations/workload.openshift.io~1allowed"},
            {"op": "add", "path": "/metadata/labels/kubernetes.io~1metadata.name", "value": "openshift-storage"},
            {"op": "add", "path": "/metadata/labels/olm.operatorgroup.uid~1ffcf3f2d-3e37-4772-97bc-983cdfce128b", "value": ""},
            {"op": "add", "path": "/metadata/labels/openshift.io~1cluster-monitoring", "value": "false"},
            {"op": "add", "path": "/metadata/labels/pod-security.kubernetes.io~1audit", "value": "privileged"},
            {"op": "add", "path": "/metadata/labels/pod-security.kubernetes.io~1audit-version", "value": "v1.24"},
            {"op": "add", "path": "/metadata/labels/pod-security.kubernetes.io~1warn", "value": "privileged"},
            {"op": "add", "path": "/metadata/labels/pod-security.kubernetes.io~1warn-version", "value": "v1.24"},
            {"op": "add", "path": "/metadata/labels/security.openshift.io~1scc.podSecurityLabelSync", "value": "true"},
            {"op": "add", "path": "/spec", "value": {"finalizers": {{ .spec.finalizers | toJson }} }}
        ]'
    }
```

It must be possible to decode the content of the `patch` field as json.
`kind`, `apiVersion`, `name` and `namespace` are corrilation fields and are used to match the patch with the correct cluster CR.
If for some reason this is not working you can use the `exactMatch` field which works the same way as the `manual correlation` for templates.
In this example you would add `exactMatch: v1_Namespace_openshift-storage`.

Note in the `go-template` the patch is required to generate a patch defintion when the cluster CR is passed in as the agumment to the template.
However, only the `type` and `patch` are required - the reason and any corrilation fields will be taken from the inital patch.
