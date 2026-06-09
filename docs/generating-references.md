# Generating References

A user may wish to generate a reference from a running cluster or from an `oc adm must-gather` output.
Once a cluster is deployed and all operators are installed and configured, generating a reference from that cluster will allow users to quickly compare and validate other clusters against it.
This can be useful when working on a specific blueprint or other application-specific deployment.
It can also be helpful in identifying any configuration drift in the cluster, if that output is later ran against the same source cluster.

This is achieved by first creating a configuration file outlining what to capture, then running the tool with the required options.

## Creating A Generator Configuration File

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

Runtime annotations and labels are automatically removed, however additional fields can be specified via the `omitAnnotations` and `omitLabels` parameters.

## Generating a Reference Configuration

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

The `metadata.yaml` file conforms to what `kubectl cluster-compare -r` expects:

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

If necessary, `metadata.yaml` can be further modified e.g. to add or remove fields to omit, or changing resource groupings from `allOf` to `anyOf` to suit a particular use case.

See [reference-config-guide-v2.md](./reference-config-guide-v2.md) for more details.
