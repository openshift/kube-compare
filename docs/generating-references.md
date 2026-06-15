# Generating References

A reference configuration is a directory containing a `metadata.yaml` and one YAML file per captured object. You can build that directory by hand, or bootstrap it from a running cluster or an `oc adm must-gather` archive using the `-g` generate option of the `cluster-compare` tool.

Once a cluster is deployed and operators are configured, a generated reference lets you compare and validate other clusters against it. This is useful for blueprints, application-specific deployments, and drift detection on the source cluster itself.

The sections below describe the generator configuration file and the `-g` command. The first section describes how generation fits in the full reference workflow.

## Overview

Defining a robust reference is an iterative process. In general, the flow is:

1. **Collect** the specific objects that are critical to the given blueprint or application.
2. **Create `metadata.yaml`** to group those objects and define which are optional versus required.
3. **Add exclusion rules** to ignore annotations, labels, and other runtime fields that are not germane to the application or blueprint.
4. **Edit the captured objects** to inject template stanzas that define allowable variations.
5. Iterate/repeat.

As the reference is run against additional clusters, false positives can be reduced by iterating on the steps above:

- Adjusting `metadata.yaml` to refine which objects are optional or how they are grouped (`allOf`, `anyOf`, etc.).
- Adding field omissions to ignore additional labels, annotations, or other fields.
- Adding templates to individual objects to define allowable variations and other enhanced comparison behavior. See [reference-config-guide-v2.md](./reference-config-guide-v2.md) for details.

The **Generate** feature (`kubectl cluster-compare -g`) automates steps 1–3 to bootstrap an initial reference. Step 4 and further iteration are manual refinements after generation.

| Workflow step | Automated by `-g`? | Where in this document |
|---------------|--------------------|-------------------------|
| Collect critical objects | Yes | [Generator config `resources`](#creating-a-generator-configuration-file) |
| Create `metadata.yaml` | Yes | [Generated output](#generated-output) |
| Add exclusion rules | Yes (defaults + optional overrides) | [`omitAnnotations` / `omitLabels`](#creating-a-generator-configuration-file), [generated `fieldsToOmit`](#generated-output) |
| Inject template stanzas | No | [Next steps](#next-steps-after-generation) → [reference-config-guide-v2.md](./reference-config-guide-v2.md) |
| Iterate on false positives | Partially (re-run or hand-edit) | [Next steps](#next-steps-after-generation) |

## Creating a generator configuration file

This file corresponds to **workflow steps 1 and 3**: which objects to collect, which are required or optional, and any extra annotation/label keys to omit beyond the built-in defaults.

Here is the basic structure of generate config:

```yaml
# Example generate config for cluster-compare -g
# Use: kubectl cluster-compare -g docs/example/generate-config.yaml
# Or with must-gather: kubectl cluster-compare -g docs/example/generate-config.yaml -f ./must-gather.123456
apiVersion: refgen/v1
outputDir: ./generated-reference

# Optional: extra metadata.annotations / metadata.labels keys to strip from captured
# YAML and to register in generated metadata.yaml fieldsToOmit (defaults still apply).
# omitAnnotations:
#   - my.company/last-synced
# omitLabels:
#   - ci-build-id

resources:
  - kind: Namespace
    apiVersion: v1
    required: true
    names:
      - openshift-sriov-network-operator
      - openshift-ptp

  - kind: Network
    apiVersion: operator.openshift.io/v1
    required: true

  - kind: ConfigMap
    apiVersion: v1
    required: false
    namespace: openshift-operator-lifecycle-manager
    names:
      - collect-profiles-config

  - kind: SriovNetworkNodePolicy
    apiVersion: sriovnetwork.openshift.io/v1
    required: true
    namespace: openshift-sriov-network-operator

  - kind: SriovNetwork
    apiVersion: sriovnetwork.openshift.io/v1
    required: true
    namespace: openshift-sriov-network-operator

  - kind: SriovOperatorConfig
    apiVersion: sriovnetwork.openshift.io/v1
    required: true
    namespace: openshift-sriov-network-operator

  - kind: SriovFecClusterConfig
    apiVersion: sriovfec.intel.com/v2
    required: true
    namespace: vran-acceleration-operators

  # Node Tuning Operator resources
  - kind: PerformanceProfile
    apiVersion: performance.openshift.io/v2
    required: true

```

Resources should be fully defined, with an `apiVersion` and `kind` specified. To mark a resource as optional, set `required: false`. The `metadata.yaml` and YAML files will be created under the location specified under `outputDir`.

Runtime metadata (for example `status`, `resourceVersion`, `uid`, and common OpenShift-injected annotations and labels) is stripped automatically and registered in the generated `metadata.yaml` under `fieldsToOmit`. Use `omitAnnotations` and `omitLabels` to extend that list for application-specific keys.

## Generating a reference

The tool is invoked when the `-g <config.yaml>` option is passed to `kubectl cluster-compare`:

```bash
kubectl cluster-compare -g ./refgen-config.yaml
```

for a live cluster, or

```bash
kubectl cluster-compare -g ./refgen-config.yaml -f ./must-gather.123456
```

to create a reference configuration from the must-gather.

Some helpful information will also be displayed on the console:

```bash
$ kubectl cluster-compare -g my-config.yaml
  Total resources captured: 22
  Resource types: 7
Warning: No resources found for:
  - SriovFecClusterConfig (sriovfec.intel.com/v2) in namespace vran-acceleration-operators
```

## Generated output

This corresponds to **workflow step 2**: a `metadata.yaml` with `parts`/`components` groupings and a set of captured CR files. Generated YAML files are plain snapshots; they do not yet include template stanzas (step 4).

The structure of the output matches what `kubectl cluster-compare -r` expects:

```bash
generated-reference/
├── ConfigMap
│   └── collect-profiles-config.yaml
├── metadata.yaml
├── Namespace
│   ├── openshift-ptp.yaml
│   └── openshift-sriov-network-operator.yaml
├── Network
│   └── cluster.yaml
├── PerformanceProfile
│   └── openshift-node-performance-profile.yaml
├── SriovNetwork
│   ├── sriov-network1.yaml
│   └── sriov-network2.yaml
├── SriovNetworkNodePolicy
│   ├── pci-sriov1.yaml
│   └── pci-sriov2.yaml
└── SriovOperatorConfig
    └── default.yaml
```

Note that re-running the tool will not overwrite already generated resource files, but will create new files with a number appended to it, e.g. `generated-reference/PerformanceProfile/openshift-node-performance-profile-1.yaml`.

Here's what a generated `metadata.yaml` file should look like:

```yaml
apiVersion: v2
fieldsToOmit:
  defaultOmitRef: all
  items:
    all:
    - include: defaults
    - pathToKey: status
    defaults:
    - pathToKey: metadata.annotations."kubernetes.io/metadata.name"
    - pathToKey: metadata.annotations."openshift.io/sa.scc.uid-range"
    - pathToKey: metadata.annotations."openshift.io/sa.scc.mcs"
    - pathToKey: metadata.annotations."openshift.io/sa.scc.supplemental-groups"
    - pathToKey: metadata.annotations."machineconfiguration.openshift.io/mc-name-suffix"
    - pathToKey: metadata.annotations."kubectl.kubernetes.io/last-applied-configuration"
    - pathToKey: metadata.annotations."nmstate.io/webhook-mutating-timestamp"
    - pathToKey: metadata.annotations."ran.openshift.io/ztp-gitops-generated"
    - pathToKey: metadata.annotations."include.release.openshift.io/ibm-cloud-managed"
    - pathToKey: metadata.annotations."include.release.openshift.io/self-managed-high-availability"
    - pathToKey: metadata.annotations."include.release.openshift.io/single-node-developer"
    - pathToKey: metadata.annotations."release.openshift.io/create-only"
    - pathToKey: metadata.annotations."capability.openshift.io/name"
    - pathToKey: metadata.annotations."olm.providedAPIs"
    - pathToKey: metadata.annotations."operator.sriovnetwork.openshift.io/last-network-namespace"
    - pathToKey: metadata.annotations."k8s.v1.cni.cncf.io/resourceName"
    - pathToKey: metadata.annotations."security.openshift.io/MinimallySufficientPodSecurityStandard"
    - pathToKey: metadata.labels."kubernetes.io/metadata.name"
    - isPrefix: true
      pathToKey: metadata.labels."pod-security.kubernetes.io/"
    - isPrefix: true
      pathToKey: metadata.labels."operators.coreos.com/"
    - pathToKey: metadata.labels."security.openshift.io/scc.podSecurityLabelSync"
    - pathToKey: metadata.labels."lca.openshift.io/target-ocp-version"
    - pathToKey: metadata.labels."olm.operatorgroup.uid"
    - pathToKey: metadata.resourceVersion
    - pathToKey: metadata.uid
    - pathToKey: metadata.creationTimestamp
    - pathToKey: metadata.generation
    - pathToKey: metadata.finalizers
    - pathToKey: metadata.ownerReferences
    - pathToKey: spec.finalizers
    - pathToKey: spec.ownerReferences
    - pathToKey: spec.clusterID
    - pathToKey: spec.filters
parts:
- components:
  - allOf:
    - path: Namespace/openshift-sriov-network-operator.yaml
    - path: Namespace/openshift-ptp.yaml
    name: namespace
  description: Required Namespace resources
  name: required-namespace
- components:
  - allOf:
    - path: Network/cluster.yaml
    name: network
  description: Required Network resources
  name: required-network
- components:
  - anyOf:
    - path: ConfigMap/collect-profiles-config.yaml
    name: configmap
  description: Optional ConfigMap resources
  name: optional-configmap
- components:
  - allOf:
    - path: SriovNetworkNodePolicy/pci-sriov1.yaml
    - path: SriovNetworkNodePolicy/pci-sriov2.yaml
    name: sriovnetworknodepolicy
  description: Required SriovNetworkNodePolicy resources
  name: required-sriovnetworknodepolicy
- components:
  - allOf:
    - path: SriovNetwork/sriov-network1.yaml
    - path: SriovNetwork/sriov-network2.yaml
    name: sriovnetwork
  description: Required SriovNetwork resources
  name: required-sriovnetwork
- components:
  - allOf:
    - path: SriovOperatorConfig/default.yaml
    name: sriovoperatorconfig
  description: Required SriovOperatorConfig resources
  name: required-sriovoperatorconfig
- components:
  - allOf:
    - path: PerformanceProfile/openshift-node-performance-profile.yaml
    name: performanceprofile
  description: Required PerformanceProfile resources
  name: required-performanceprofile
```

## Next steps after generation

Generation produces a usable starting point, but not necessarily a finished reference. A typical follow-up work-flow consists of:

1. **Run a comparison** against the source cluster to confirm the bootstrap is clean:

   ```bash
   kubectl cluster-compare -r ./generated-reference/metadata.yaml
   ```

2. **Compare against other clusters** and note false positives (unexpected diffs that are acceptable variation).

3. **Iterate** using the same levers as in [Overview](#overview):
   - Edit `metadata.yaml` — change optional/required groupings, switch `allOf` to `anyOf`, add `fieldsToOmit` entries.
   - Re-run `-g` with an updated generator config, or hand-edit captured YAML under the output directory (existing files are not overwritten; new captures get a numeric suffix).
   - Add template annotations and stanzas to individual CR files for allowable variation, correlation overrides, and other comparison behavior.

For template syntax, grouping semantics, and advanced comparison rules, see [reference-config-guide-v2.md](./reference-config-guide-v2.md).
