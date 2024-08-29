
# Reference Configuration

The reference configuration consists of a metadata.yaml file and a set of CRs (yaml files with one CR per file). The
metadata.yaml includes higher level logic, settings that are about the connection between multiple of crs and the CRs
files contain lower level logic, validation rules on how to compare each templated CR with its matching cluster CR.

## Reference Configuration CRs

The reference configuration CRs conform to
these criteria:

1. Each CR may contain annotations to indicate fields which have expected user variation or which
   are optional.
1. Any field of a reference configuration CR which is not otherwise annotated is required and the
   value must be as specified in order to be compliant.

### Reference CR Groupings

The most important thing included in the metadata.yaml is the list of all the CR templates that are included in the
reference. The file includes an hierarchy of all the CR templates by parts and components. Parts are Groups of
components and components are groups of CR templates.

Each component includes required templates and optional templates. Required templates are CRs that must be present in
the input configuration. They are reported in the summary as "missing content" in case there are no cluster CRs that
correlate with the required template. Optional CRs will not be reported in the summary as "missing content".

A component can also be flagged as optional, in this case if any of the
required CRs are included in the input configuration then all required CRs
in the group are expected to be included and any which are missing will be reported. If none of
the required CRs in the group are included then no report of "missing content" for the group will be generated.

In this version Parts only help with organization of the components into groups and don't have any affection on the diff
process.

Thus, the file `metadata.yaml` includes an array denoted by `parts` of one or more objects. Each object includes:

- a key "name" with a value of a string typically identifying a workload or a set of workloads
- a key "components" defined as an array of objects

```yaml
# Every part denotes typically denotes a workload or set of workloads
 parts:
  - name: ExamplePart1
    components:
      - name: ExampleComponent1
        ---- here goes ExampleComponent1 configuration ----
  - name: ExamplePart2
    ---- here goes Part2 configuration ----
```

"components" array includes:

- a pair at key "name" with a value of a string identifying a component required for the part
- a pair at key "type" with a value of:
  - `Optional` for optional components; or
  - `Required` for mandatory components
- the key "requiredTemplates" defined by:
  - an array of at least one template `YAML` files. Each template `YAML` file has one Reference Configuration CR.

```yaml
# requiredTemplates contains all needed reference CRs to comply with ExampleComponent1
components:
  - name: ExampleComponent1
    type: Optional
    requiredTemplates:
      - path: RequiredTemplate1.yaml
      - path: RequiredTemplate2.yaml
```

### Example Reference Configuration CR

User variable content is handled by golang formatted templating within the reference configuration
CRs. This templating format allows for simple "any value", complex validation, and conditional
inclusion/exclusion of content. The following types of user variation are expected to be handled:

1. Mandatory user-defined fields. Examples are marked #1.
1. Optional user-defined fields. Examples are marked #2
1. Validation of user defined fields. Examples are marked with #3

GO templating allows use of custom and built in functions to allow complex use cases. In this version all Go built-in
functions are supported along with the functions in the Sprig library. Also this version follows the Helm templating
behavior and supports all custom functions that are used in helm (example: toYaml).

```yaml
apiVersion: v1
kind: Service
metadata:
  name: frontend
  namespace: {{ .metadata.namespace }}  #1 mandatory user variable content
  labels:
    app: guestbook
    tier: frontend
spec:
  # if you want to use a LoadBalancer and your cluster supports it, use LoadBalancer else use NodePort
  type:
  {{- if and .spec.type (or (eq (.spec.type) "NodePort") (eq (.spec.type) "LoadBalancer")) }} {{.spec.type }} # 3 validates type
  {{- else }} should be NodePort or LoadBalancer
  {{- end }}
  ports:
  - port: 80
  selector:
    app: guestbook
    {{- if .sepc.selector.tier }} #2 optional fields
    tier: frontend
    {{- end }}
```

# Per-template configuration

## Pre-merging

If you don't want to check live-manifest exactly matches your template you can enable merging.
This will do a strategic merge of the template into the manifest which will remove features not mentioned in the template from the diff.
This can be useful when dealing with annotation or labels which may be of no consiquence to your check.
Note that this could cover up differences in things you do care about so use it with care.
This can be configured for a manifest by adding config to the metadata.yaml

```yaml
parts:
  - name: ExamplePart
    components:
      - name: Namespace
        type: Required
        requiredTemplates:
          - path: namespace.yaml
            config:
              ignore-unspecified-fields: true
```

example when comparing the template:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: openshift-storage
  annotations:
    workload.openshift.io/allowed: management
  labels:
    openshift.io/cluster-monitoring: "true"
```

to the manifest:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  annotations:
    openshift.io/sa.scc.mcs: s0:c29,c14
    openshift.io/sa.scc.supplemental-groups: 1000840000/10000
    openshift.io/sa.scc.uid-range: 1000840000/10000
    reclaimspace.csiaddons.openshift.io/schedule: '@weekly'
  creationTimestamp: "2024-06-07T17:40:07Z"
  labels:
    kubernetes.io/metadata.name: openshift-storage
    olm.operatorgroup.uid/ffcf3f2d-3e37-4772-97bc-983cdfce128b: ""
    pod-security.kubernetes.io/audit: privileged
    pod-security.kubernetes.io/audit-version: v1.24
    pod-security.kubernetes.io/warn: privileged
    pod-security.kubernetes.io/warn-version: v1.24
    security.openshift.io/scc.podSecurityLabelSync: "true"
  name: openshift-storage
  resourceVersion: "13323419"
  uid: 507a5a4e-4fca-4dc3-b246-36359cbe07bf
spec:
  finalizers:
  - kubernetes
status:
  phase: Active
```

you will get a diff of:

```shell
**********************************

Cluster CR: v1_Namespace_openshift-storage
Reference File: namespace.yaml
Diff Output: diff -u -N TEMP/v1_namespace_openshift-storage TEMP/v1_namespace_openshift-storage
--- TEMP/v1_namespace_openshift-storage DATE
+++ TEMP/v1_namespace_openshift-storage DATE
@@ -6,11 +6,9 @@
     openshift.io/sa.scc.supplemental-groups: 1000840000/10000
     openshift.io/sa.scc.uid-range: 1000840000/10000
     reclaimspace.csiaddons.openshift.io/schedule: '@weekly'
-    workload.openshift.io/allowed: management
   labels:
     kubernetes.io/metadata.name: openshift-storage
     olm.operatorgroup.uid/ffcf3f2d-3e37-4772-97bc-983cdfce128b: ""
-    openshift.io/cluster-monitoring: "true"
     pod-security.kubernetes.io/audit: privileged
     pod-security.kubernetes.io/audit-version: v1.24
     pod-security.kubernetes.io/warn: privileged

**********************************

Summary
CRs with diffs: 1
No CRs are missing from the cluster
No CRs are unmatched to reference CRs
```

instead of

```shell
**********************************

Cluster CR: v1_Namespace_openshift-storage
Reference File: namespace.yaml
Diff Output: diff -u -N TEMP/v1_namespace_openshift-storage TEMP/v1_namespace_openshift-storage
--- TEMP/v1_namespace_openshift-storage DATE
+++ TEMP/v1_namespace_openshift-storage DATE
@@ -2,7 +2,19 @@
 kind: Namespace
 metadata:
   annotations:
-    workload.openshift.io/allowed: management
+    openshift.io/sa.scc.mcs: s0:c29,c14
+    openshift.io/sa.scc.supplemental-groups: 1000840000/10000
+    openshift.io/sa.scc.uid-range: 1000840000/10000
+    reclaimspace.csiaddons.openshift.io/schedule: '@weekly'
   labels:
-    openshift.io/cluster-monitoring: "true"
+    kubernetes.io/metadata.name: openshift-storage
+    olm.operatorgroup.uid/ffcf3f2d-3e37-4772-97bc-983cdfce128b: ""
+    pod-security.kubernetes.io/audit: privileged
+    pod-security.kubernetes.io/audit-version: v1.24
+    pod-security.kubernetes.io/warn: privileged
+    pod-security.kubernetes.io/warn-version: v1.24
+    security.openshift.io/scc.podSecurityLabelSync: "true"
   name: openshift-storage
+spec:
+  finalizers:
+  - kubernetes

**********************************

Summary
CRs with diffs: 1
No CRs are missing from the cluster
No CRs are unmatched to reference CRs
```

## Ignoring feilds

It is possible as a reference writter to ignore fields for a given template.

First you must define an entry in `fieldsToOmit.items` e.g.:

```yaml
fieldsToOmit:
   items:
      deployments:
         - pathToKey: spec.selector.matchLabels.k8s-app # remove spec.selector.matchLabels.k8s-app before diff
```

This can then be referenced in the entry for the template:

```yaml
requiredTemplates:
  - path: redis-master-deployment.yaml
    config:
        fieldsToOmitRefs:
          - deployments
```

> Note: setting `fieldsToOmitRefs` will replace the default value.

`fieldsToOmit` can define a default value for `fieldsToOmitRefs` using the key `defaultOmitRef`:

```yaml
fieldsToOmit:
   defaultOmitRef: default
   items:
      defualt:
         - pathToKey: a.custom.default."k8s.io" # Keys containing dots should be quoted
```

The default value of `defaultOmitRef` is a built-in list  `cluster-compare-built-in` and can still be referenced even if the `defaultOmitRef` is set.

### pathToKey syntax

The syntax for `pathToKey` is a dot seperated path.

> Limitation: currently we are not able to traverse lists.

The path: `"spec.selector.matchLabels.k8s-app"` will match:

```yaml
spec:
  selector:
    matchLabels:
      k8s-app: "..."
```

Segements with dots should be quoted. So to match:

```yaml
metadata:
  annotations:
    workload.openshift.io/allowed: management
```

you use would use `metadata.annotations."workload.openshift.io/allowed"`.

# Catch all templates

It is possible to create catch all templates to manifests not corrilated by others.
This is becuase the more specific templates are prefered of less specific ones.
So by adding wildcards of the corrilated fields such as name or namespace,
you can have templates that will match manifests not caught more specific templates.
In our test data we have an example of using [`MachineConfigs`](../pkg/compare/testdata/MachineConfigsCatchAll/reference/)
