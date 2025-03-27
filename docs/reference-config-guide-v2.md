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
apiVersion: v2 # Required will default to v1
parts:
  - name: ExamplePart1
    components:
      - name: ExampleComponent1
        ---- here goes ExampleComponent1 configuration ----
  - name: ExamplePart2
    ---- here goes Part2 configuration ----
```

"components" array includes objects with the following keys:

- a pair at key "name" with a value of a string identifying a component required for the part
- one of the following:
  - `allOf`: Will cause a validation error if not all the templates are matched. Use for sets of required templates.
  - `allOrNoneOf`: Will cause a validation error when some but not all of the templates are matched. Use for sets of templates where the entire set is optional, but if at least one is present then all the others must also be present.
  - `anyOf`: Will only cause validation errors for the content of matched templates. Use for completely optional templates.
  - `noneOf`: Will cause a validation error if any of the templates are matched. Use for excluding CRs.
  - `oneOf`: Will cause a validation error when none or more than one of the templates is matched. For requiring exactly one template in the set.
  - `anyOneOf`: Will throw a validation error if more than one template is matched. Use for optionally allowing only one of the templates in the set.

```yaml
components:
  - name: ExampleComponent1
    allOf:
      - path: RequiredTemplate1.yaml
      - path: RequiredTemplate2.yaml
  - name: ExampleComponent2
    allOrNoneOf:
      - path: OptionalBlockTemplate1.yaml
      - path: OptionalBlockTemplate2.yaml
  - name: ExampleComponent3
    anyOf:
      - path: OptionalTemplate1.yaml
      - path: OptionalTemplate2.yaml
  - name: ExampleComponent4
    noneOf:
      - path: BannedTemplate1.yaml
      - path: BannedTemplate2.yaml
  - name: ExampleComponent5
    oneOf:
      - path: RequiredExclusiveTemplate1.yaml
      - path: RequiredExclusiveTemplate2.yaml
  - name: ExampleComponent6
    anyOneOf:
      - path: OptionalExclusiveTemplate1.yaml
      - path: OptionalExclusiveTemplate2.yaml
```

### Reference Descriptions

In order to make detected differences more actionable, each part, component,
and template may include a description section which is displayed when a
difference is detected, or if a required template is absent.

The description section is a free-form multi-line text field which can contain
instructions or a URL referencing extra documentation about the template in
question, such as why it is required or which fields are optional.

Only one description is shown per template, and more specific descriptions
override those which are less specific. In other words, we display whichever of
these 3 matches first:

1. Template description, if set
2. Component description, if set
3. Part description, if set

Example metadata.yaml with descriptions:

```yaml
apiVersion: v2
parts:
  - name: ExamplePart1
    description: |-
      General text for every template beneath, unless overridden.
    components:
      - name: ExampleComponent1o
        # With no description set, this inherits that of the part above.
        OneOf:
          - path: Template1.yaml
            # Likewise this inherits the component description
          - path: Template2.yaml
          - path: Template3.yaml
            description: |-
              This template has special instructions that don't apply to the others.
      - name: ExampleComponent2
        description: |-
          This overrides the part text with something more specific.
          Multi-line text is supported, at all levels.
        allOf:
          - path: RequiredTemplate1.yaml
          - path: RequiredTemplate2.yaml
            description: |-
              Required for Important Reasons.
          - path: RequiredTemplate3.yaml
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

### Additional template functions

These custom functions add capabilities beyond the capabilities of the sprig or
helm templates.

#### lookupCRs

Search for external structured objects.

Usage:

```yaml
{{- $objlist := lookupCRs "$apiVersion" "$kind" "$namespace" "$name" }}
```

Returns a list of matching objects.  The `$apiVersion` and `$kind` arguments are
required.  Both `$namespace` and `$name` can be blank or have the value `*`
to match any namespace or name.

This can be used within a template to represent a relationship between the
object being matched and another separate object elsewhere.  Note: Only other
objects that have templates to match in the current reference will be found -
This cannot look up arbitrary objects on a cluster.  Likewise, when running in
offline mode (with the `-f` option), only objects already in the file system
paths specified are in scope to be found by this function.

#### lookupCR

Search for a singular structured object.

Usage:

```yaml
{{- $obj := lookupCR "$apiVersion" "$kind" "$namespace" "$name" }}
```

or

```yaml
field: {{ (lookupCR "$apiVersion" "$kind" "$namespace" "$name").spec.replicas }}
```

Returns the matching object if there is exactly one.  The `$apiVersion` and
`$kind` arguments are required.  Both `$namespace` and `$name` can be blank or
have the value `*` to match any namespace or name.  If multiple objects are
matched, this returns `nil`.

## Per-template configuration

### Pre-merging

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
        allOf:
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

### Ignoring feilds

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
      default:
         - pathToKey: a.custom.default."k8s.io" # Keys containing dots should be quoted
```

The default value of `defaultOmitRef` is a built-in list  `cluster-compare-built-in` and can still be referenced even if the `defaultOmitRef` is set.

#### Referencing field omission groups

A group of field omissions may reference other groups of field omission items to allow less duplication in group creation. For example:

```yaml
fieldsToOmit:
   defaultOmitRef: default
   items:
    common:
      - pathToKey: metadata.annotations."kubernetes.io/metadata.name"
      - pathToKey: metadata.annotations."kubernetes.io/metadata.name"
      - pathToKey: metadata.annotations."kubectl.kubernetes.io/last-applied-configuration"
      - pathToKey: metadata.creationTimestamp
      - pathToKey: metadata.generation
      - pathToKey: spec.ownerReferences
      - pathToKey: metadata.ownerReferences
    default:
      - include: common
      - pathToKey: status
```

#### pathToKey syntax

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

### PerField Configuration

#### Inline Diff Funcs

This Version of the tool contains builtin Inline diff functions. Inline diff functions are functions that run on a specific
field in the templates. The functions contain common advanced diff functions that may be also implemented with pure go templating
but if they were implemented with go templating would result in unclear diff logic that is hard to maintain.
The functions goal is to enable a more clear and readable usage for common custom templating functions and allow more complex
functionalities.

To specify an inline function for a specific field yse the `perField` section in the metadata.yaml file as in this example:

```yaml
apiVersion: v2
parts:
- name: ExamplePart
  components:
  - name: Example
    allOf:
    - path: cm.yaml
      config:
        perField:
        - pathToKey: spec.bigTextBlock # Field in template to run the inline diff function in pathToKey syntax
          inlineDiffFunc: regex # Inline function
```

Supported inline diff functions:

##### Regex Inline Diff Function

The `regex` inline diff function allows validating fields in CRs based on a
regex. When using the function the command will show no diffs in case the
cluster CR will match the regex, if it does not match the regex a diff will be
shown between the cluster CR and the regex expression. To use the regex inline
diff function you need to enable the regexInline function for the specific
field and template in the metadata.yaml and also specify the regex inside the
template.

Additionally, we validate that all identically-named capturegroups in the regx
match the same values.  See "Enforcing named capturegroup values" below for
details and how these intersect with the `capturegroup` inline diff function.

For a template named cm.yaml where spec.bigTextBlock should be validated by regex:

```yaml
kind: ConfigMap
apiVersion: v1
metadata:
  namespace: kubernetes-dashboard
data:
  bigTextBlock: |-
    This is a big text block with some static content, like this line.
    It also has a place where (?<username>[a-z0-9]+) would put in their own name. (?<username>[a-z0-9]+) would put in their own name.
```

the metadata.yaml should contain:

```yaml
apiVersion: v2
parts:
- name: ExamplePart
  components:
  - name: Example
    allOf:
    - path: cm.yaml
      config:
        perField:
        - pathToKey: data.bigTextBlock
          inlineDiffFunc: regex
```

##### Capturegroup Inline Diff Function

The `capturegroups` inline diff function is related to the `regex` inline diff,
but is better suited for maintaining readability and comparisons of multi-line
strings.  Instead of the entire template value being treated as a single
regular expression, it treats any text outside of a regex-format named capture
group (ie, something like `(?<somename>...)`) as text that must match exactly,
only performing regular expression matching within the individual capturegroups.

When rendering the template document, we also apply a diff algorithm to attempt
to retain the maximum amount of text in common between the template and the
object being compared, and then we attempt to reconcile any outstanding
differences by attempting to match the capturegroups in the template with the
corresponding text in the object being compared.  This is inexact, but does a
better job than the plain `regex` especially for larger multi-line text blocks.

Additionally, we validate that all identically-named capturegroups match the
same values.  See "Enforcing named capturegroup values" below for details and how
these intersect with the `regex` inline diff function.

For a template named cm.yaml where spec.bigTextBlock should be validated by
capturegroups:

```yaml
kind: ConfigMap
apiVersion: v1
metadata:
  namespace: kubernetes-dashboard
data:
  bigTextBlock: |-
    This is a big text block with some static content, like this line.
    It also has a place where (?<username>[a-z0-9]+) would put in their own name. (?<username>[a-z0-9]+) would put in their own name.
```

the metadata.yaml should contain:

```yaml
apiVersion: v2
parts:
- name: ExamplePart
  components:
  - name: Example
    allOf:
    - path: cm.yaml
      config:
        perField:
        - pathToKey: data.bigTextBlock
          inlineDiffFunc: capturegroups
```

##### Enforcing named capturegroup values

Within a single object template, we additionally validate that all
identically-named capturegroups match the same values across multiple fields,
and this is checked regardless of whether they use the `regex` or
`capturegroups` inline diff functionality.

For example, an object as follows:

```yaml
kind: ConfigMap
apiVersion: v1
metadata:
  namespace: kubernetes-dashboard
data:
  username: "(?<username>[a-z0-9]+)"
  bigTextBlock: |-
    This is a big text block with some static content, like this line.
    It also has a place where (?<username>[a-z0-9]+) would put in their own name. (?<username>[a-z0-9]+) would put in their own name.
```

With a metadata.yaml:

```yaml
apiVersion: v2
parts:
- name: ExamplePart
  components:
  - name: Example
    allOf:
    - path: cm.yaml
      config:
        perField:
        - pathToKey: data.username
          inlineDiffFunc: regex
        - pathToKey: data.bigTextBlock
          inlineDiffFunc: capturegroups
```

The inlineDiff functionality will enforce that the same username value is used
in both the `username` and `bigTextBlock` fields.

## Catch all templates

It is possible to create catch all templates to manifests not corrilated by others.
This is becuase the more specific templates are prefered of less specific ones.
So by adding wildcards of the corrilated fields such as name or namespace,
you can have templates that will match manifests not caught more specific templates.
In our test data we have an example of using [`MachineConfigs`](../pkg/compare/testdata/MachineConfigsCatchAll/reference/)
