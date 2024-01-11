# Cluster Configuration Validation Tool

## Summary

The “oc adm compare” subcommand is capable of performing an intelligent diff between a reference configuration and the specific configuration applied to a cluster. The comparison is capable of suppressing diffs of content which is expected to be user variable, validating required and optional configuration, and ignoring known runtime variable fields. With these capabilities a cluster administrator, solutions architect, support engineers, and others can validate a cluster’s configuration against a baseline reference configuration.

In addition to the subcommand to perform this comparison, this enhancement defines the structure and method of capturing known user variation, optional components, and required content in the reference configuration.

## Motivation

Many deployed clusters are based on engineered and validated reference configurations. These
reference configurations have been designed to ensure a cluster will meet the functional, feature,
performance and resource requirements for specific use cases. A customer will take this reference
configuration and adapt it for their particular environment adding variations to account for their
networking topology, specific servers/hardware in use, optional features, etc. This adapted
version of the configuration is then applied to their cluster, or replicated across a large
scale deployment of clusters. When this adapted configuration deviates from the reference
configuration the impacts may be subtle, transient, or delayed for some period of time. When working
with these clusters across their lifetimes it is important to be able to validate the configuration
against the known valid reference configuration to identify potential issues before they impact end
users, service level agreements, or cluster uptime.

This oc subcommand is capable of doing an "intelligent" diff between a reference
configuration and a set of CRs representative of a deployed production cluster. These CRs may derive
from many potential sources such as being pulled from a live cluster, extracted from a support archive, or shared directly by the customer. The reference configuration is the engineered set of configuration CRs for the use case and has been sufficiently annotated to describe
expected user variations versus required content.

```
┌──────────────────┐                 ┌──────────────────┐
│                  │ Adaptation to   │                  │
│     Published    │    user env     │   Deployed user  │
│     Reference    ├────────────────►│   Configuration  │
│   Configuration  │                 │                  │
│                  │                 │                  │
└────────┬─────────┘                 └─────────┬────────┘
         │                                     │
         │                                     │
         │        ┌──────────────────┐         │
         │        │                  │         │
         └───────►│ Proposed Cluster │◄────────┘
                  │ Validation Tool  │
                  │                  │
                  │                  │
                  └─────────┬────────┘
                            │
                       ┌────▼────┐
                       │Relevant │
                       │Diffs    │
                       │         │
                       │...      │
                       │...      │
                       │...      │
                       └─────────┘
```
Existing tools meet some of this need but fall short of the goals
- kubectl/oc diff: This subcommand allows comparison of a live cluster against a known configuration.
  There are three shortcomings we need to address:
    - Ability to handle expected user variation and optional versus required content.
    - Ability to handle 1 to N mappings where users have multiple instances of a CR which should be validated
    - Consumption of an offline representation of the cluster configuration.
- `<get cr> | <key sorting> | diff` : There are various ways of chaining together existing tools
  to obtain, correlate, and compare/diff two YAML objects. These methods fall short in similar ways as the `kubectl/oc diff`

### Goals

The design and implementation of this subcommand is guided by the following goals:
1. Data driven. New and updated reference configurations do not require a new release/update of the command.
1. New and updated reference configurations can be “published” asynchronously to the tooling
1. Can consume input configuration CRs from live cluster, the local filesystem, or a support archive (future enhancement)
1. Will suppress diffs for runtime variable fields (status, managed metadata, etc)
1. Will suppress diffs for know user variable content as described by the reference configuration
1. Will show diffs for content in input configuration which does not match reference
1. Will show diffs for reference configuration content missing from input configuration
1. Will show diffs for content in input configuration which is not contained in reference
1. Allows comparison against 1 to N mappings in the reference configuration (ie an input configuration CR compared against one of several optional implementations in the reference configuration)
1. Allows comparison against 1 to N mappings to the reference configuration (ie multiple input configuration CRs compared against one reference configuration CR)

### Non-Goals

1. Validation which goes beyond what is available through CRs accessible via the API – deeper
   inspection/analysis is the domain of other tools.
1. Validation of configuration CRs against CRD -- “linting” validation of their correctness or ability to be successfully applied to a cluster

## Proposal

### Terminology

* **drift** -- A significant delta/difference which needs to be brought into compliance or undergo further review/assessment

### Validation Tool Implementation

The validation tool will operate similarly to a standard Linux `diff` tool which operates (recursively) across a
set of inputs (eg two trees of input). The left hand side of the diff will be a user selected reference
configuration (see below for structure/contents of the reference) and the right hand side will be a
collection of the user’s configuration CRs. The logical flow of the tool will be:
1. User invokes the tool with the two inputs: `oc adm compare <referenceConfig> <userConfig>`
    1. When the tool is run against a live cluster the `<userConfig>` input is made up of the set of
       CRs pulled from the cluster based on the reference configuration. Only those CRs included in
       the reference configuration are pulled from the live cluster. Where the reference
       configuration indicates user variability in CR name or namespace multiple CRs may be pulled
       based on the kind and included in the `<userConfig>`.
    1. When the tool is run against extracted CRs `<userConfig>` is a local directory.
1. For each CR in `<userConfig>`
    1. Correlate the CR to a referenceConfig CR using api-kind-namespace-name (see [Correlating
       CRs](# Correlating-CRs) below)
    1. Generate a rendered reference CR. Expected user variable content is pulled from the input CR,
       validated, and inserted into the rendered reference CR.
    1. Perform and standard Linux `diff` between rendered reference CR and the input CR. Any
       non-expected variations and/or missing content are reported.
1. For each unused required reference CR (see [Reference CR Annotations](#
   Reference-CR-Annotations)) report missing content

As described in the logical flow the tool will report any differences considered outside the expected set
of variability as defined by the reference configuration (ie the "drift"). The tool will highlight
this drift for additional analysis/review by the user. In addition to the CR comparison output the tool will output a report detailing:
* Input configuration CRs with no match in the reference
* Required reference CRs with no match in the input configuration
* Number of drifts found

#### Inputs

The tool consumes two mandatory inputs and supports additional options to control the comparison,
output, etc.

The reference configuration is a required input. The structure of the reference is described
below. The minimum requirement is that the reference can be located on the local filesystem (eg
directory).

The user configuration is an optional input. If specified the user configuration will be pulled from the local filesystem. Otherwise the user configuration will be pulled from a live cluster.

#### Correlating CRs

`oc adm compare` must correlate CRs between reference and input configurations to perform the
comparisons. `oc adm compare` correlates CRs by using the apiVersion, kind, namespace and name fields of the CRs to perform a nearest match correlation. Optionally the user may provide a manual override of the correlation to identify a specific reference configuration CR to be used for a given user input CR. Manual matches are prioritized over the automatic of correlation, meaning manual matches override matches by similar values in the specified group of fields.

##### Correlation by manual matches

`oc adm compare` gets as input a diff config that contains an option to specify manual matches between cluster resources and resource templates. The matches can be added to the config as pairs of  apiVersion_kind_namespace_name: <Template File Name>. For cluster scoped CRs that don't have a namespace the matches can be added  as pairs of apiVersion_kind_name: <Template File Name>.


##### Correlation by group of fields (apiVersion, kind, namespace and name)

When there is no manual match for a CR the command will try to match a template for the resource by looking at the 4-tuple: apiVersion, kind, namespace and name . The Correlation is based on which fields in the templates that are not user-variable. Templates get matched to resources based on all the features from the 4-tuple that are declared fixed (not user-variable) in the templates.  
For example a template with a fixed namespace, kind, name and templated (user-variable) apiVersion will only be a potential match by the  kind-namespace-name criterion.

For each resource the group correlation will be done by the next logic:


1. Exact match of apiVersion-kind-namespace-name
    1. If single result in reference, comparison will be done
1. Exact Match in 3/4 fields from apiVersion, kind, namespace, name. ( meaning exact match in: kind-namespace-name or apiVersion-kind-name or apiVersion-kind-namespace)
    1. If single result in reference, comparison will be done
1. Exact Match in 2/4 fields from apiVersion, kind, namespace, name. ( meaning exact match in: kind-namespace or kind-name or apiVersion-kind)
    1. If single result in reference, comparison will be done
1. Match kind
    1. If single result in reference, comparison will be done
1. No match – comparison cannot be made and the file is flagged as unmatched.

We can phrase this logic in a more general form. Each CR will be correlated to a template with an exact match in the largest number of fields from this group:  apiVersion, kind, namespace, name.

#### Output

The tool will generate standard diff output highlighting content as described in "Categorization of
differences". Note in this example the cpusets and hugepage count are not highlighted as these are
expected user variations. The hugepage node is indicated as extra content and the realtime kernel
setting is indicated as a drift

```diff
@@ -8,7 +8,7 @@
   namespace: MyNamespace
 spec:
   ports:
-  - port: 8000
+  - port: 80
   selector:
     app: guestbook
     tier: frontend

---
<next CR>
…

Summary
Missing 1 required CRs:
guestbook:
  frontend:
  - frontend-deployment.yaml
No CRs are unmatched

```

### Reference Configuration Specification

The reference configuration consists of a metadata.yaml file and a set of CRs (yaml files with one CR per file). The metadata.yaml includes higher level logic, settings that are about the connection between multiple of crs and the CRs files contain lower level logic, validation rules on how to compare each templated CR with its matching  cluster CR.


##### Reference Configuration CRs

The reference configuration CRs conform to
these criteria:
1. Each CR may contain annotations to indicate fields which have expected user variation or which
   are optional.
1. Any field of a reference configuration CR which is not otherwise annotated is required and the
   value must be as specified in order to be compliant.

#### Reference CR Groupings


The most important thing included in the metadata.yaml is the list of all the CR templates that are included in the reference. The file includes an hierarchy of all the CR templates by parts and components.  Parts are Groups of Components and components are groups of CR templates.

Each component includes required templates and optional templates. Required templates are CRs that  must be present in the input configuration. They are reported in the summary as "missing content" in case there are no cluster CRs that correlate with the required template. Optional CRs will not be reported in the summary as "missing content".

A component can also be flagged as optional, in this case if any of the
required CRs are included in the input configuration then all required CRs
in the group are expected to be included and any which are missing will be reported. If none of
the required CRs in the group are included then no report of "missing content" for the group will be generated.

In this version Parts only help with organization of the components  into groups and don't have any affection on the diff process.


#### Example Reference Configuration CR

User variable content is handled by golang formatted templating within the reference configuration
CRs. This templating format allows for simple "any value", complex validation, and conditional
inclusion/exclusion of content. The following types of user variation are expected to be handled:
1. Mandatory user-defined fields. Examples are marked #1.
1. Optional user-defined fields. Examples are marked #2
1. Validation of user defined fields. Examples are marked with #3

GO templating allows use of custom and built in functions to allow complex use cases. In this version all Go built-in functions are supported along with the functions in the Sprig library. Also this version follows the Helm templating behavior and supports all custom functions that are used in helm (example: toYaml).


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
.


#### Diff

Once the validations are complete we run a diff between the user's input configuration (now
validated) CR vs the resolved template (user variable input is pulled from input config into the
resolved template). This final step is needed to error/warn user of remaining drift that validation
steps may not catch
- E.g use case: reference may have a hardcoded field such as a namespace name and the user must comply.

The primary output of this step is a side-by-side diff as shown in the output section above. To
achieve this meaningful diff the tool must do perform two operations:
1. Render the CRs into a comparable format. This involves doing a hierarchical sorting of the keys
   to ensure consistent ordering when the CRs are rendered.
1. Perform the diff



### Workflow Description

The 2 most common cases

To Compare a known valid reference configuration with a local set of CRs:

`oc compare adm -r <referenceConfigurationDirecotry> -f <inputConfiguration>`

#### Reference Configuration Directory

#### metadata.yaml

The metadata.yaml is a mandatory file for each reference config. The commands entrypoint will be looking for the metadata.yaml file in the reference directory. The name of the file is fixed and cant be changed.

The main thing included in the metadata are the list of reference CRs that are grouped by components and parts (as described in previous sections). The Parts are specified under the Parts key in the YAML and include a list of components under the Components key. The full schema can be found in the appendix.

Another parameter that can be set in the metadata.yaml file is the templateFunctionFiles. This Implementation of the command supports the declaration of nested templates in external files that then can be used in all resource templates included in the reference. All files including nested templates should be added to the list of files under the templateFunctionFiles key.


Also the metadata,yaml includes an optional field: `fieldsToOmit`. Under this key they can specify fields that should not appear in the commands output. The fields will not be reported showed in the output for all templates in the reference, meaning no need to specify them in the resource templates. The fields included will not be showed in the output even if they are specified in the resource templates. Omitted fields can be nested therefore each field is represented by a list of strings. As can be seen in the example below.

Example for metadata.yaml:

```yaml

Parts:
  - name: guestbook
    Components:
      - name: redis
        type: Required
        requiredTemplates:
          - redis-master-deployment.yaml
          - redis-master-service.yaml
        optionalTemplates:
          - redis-replica-deployment.yaml
          - redis-replica-service.yaml
      - name: frontend
        type: Required
        requiredTemplates:
          - frontend-deployment.yaml
          - frontend-service.yaml

```

#### Diff Config

The user has an option to pass a file called the diff config. The diff config includes user preferences and content that is specific to the users cluster (not like the metadata.yaml that includes only settings that are valid for the specific reference).

In the version the diff config includes an option to specify manual matches between cluster resources and resource templates. The matches can be added to the config as pairs of apiVersion_kind_namespace_name: <Template File Name>. For resources that don't have a namespace the matches can be added  as pairs of apiVersion_kind_name: <Template File Name>.
The pairs are listed in the config under correlationSettings.manualCorrelation.correlationPairs as can be seen in the example below.


```yaml

correlationSettings:
  manualCorrelation:
    correlationPairs:
      v1_Service_guestbook_frontendService: "frontend-service.yaml"

```


### Implementation Details

oc adm compare implementation includes usage of parts of code from the K8s built-in `diff` command which combines patching and an external diff tool via
`KUBECTL_EXTERNAL_DIFF`.
The command implementation includes parsing of the reference and other user passed arguments, correlation logic, template injecting, calling the diff code and summary creation.

#### Diff command interface


The command calls diff code by using the exported Differ Struct:
Definition:

```go
type Differ struct {
    From *DiffVersion
    To   *DiffVersion
}

func (d *Differ) Diff(obj Object, printer Printer, showManagedFields bool) error
func (d *Differ) Run(diff *DiffProgram) error
```

The compare command calls the differ.Diif function for each resource, adding the injected resource and the cluster resource to the files that should be included in the diff.
As seen above the differ.Diif function gets as an argument an object that matches the Object interface:

```go
type Object interface {
    Live() runtime.Object
    Merged() (runtime.Object, error)
    Name() string
}
```
The compare command includes a custom implementation of this interface. Where the Live function returns the cluster resource and the Merged function returns the injected version of the CR.
After the differ.Diff function is called for all CRs the differ.Run() is called and the diff is printed out to stdout.



### Risks and Mitigations

1. Risk of false negatives when performing comparisons – Giving the user a false indication that a
   cluster is compliant will lead to degraded performance or functionality. These could be
   introduced by bugs in the tool or reference configuration. Leveraging standard templating syntax
   and libraries for performing the analysis (parsers, template handling, comparison) mitigates the
   risk.

### Drawbacks

Existing tools can perform a diff of two CRs – This tool extends that functionality to allow for
expected variations, optional content, and detection of missing/unmatched content.

## Design Details

###Corelators Design:

The oc adm compare uses  Different Corelators to correlate between custer resources and their matching reference template.
When Designing the structure of the corealtors we tried to come up with a design that will be: easy to add additional correlation logics, and will allow chaining of different corelators.
The Corealtors are divided into 2 types:
Base corelators  - implement a specific correlation logic
Decorator corelators - corelators that wrap other corelators and add an additional behaviour.

The current version includes 2 decorator corelators: MultiCorealtor and MetricsCorelatorDecorator. And includes 2 Base corelators: ExactMatchCorelator and  GroupCorelator. (detailed information about all of them can be found below)
To allow easy chaining all the corealtors match the corelator interface: (include Errors)



In this Version the corealtors are created and initialized in the following chain:

```
                                                               ┌─────────────────────┐
                                                    <<use>>    │                     │
                                                  ┌──────────► │                     │
                                                  │            │ ExactMatchCorelator │
┌─────────────────────┐           ┌───────────────┴─────┐      │                     │
│                     │           │                     │      │                     │
│                     │           │                     │      └─────────────────────┘
│  MetricsCorelator-  ├──────────►│   MultiCorealtor    │
│      Deorator       │  <<use>>  │                     │      ┌─────────────────────┐
│                     │           │                     │      │                     │
└─────────────────────┘           └────────────────┬────┘      │                     │
                                                   │           │    GroupCorelator   │
                                                   └─────────► │                     │
                                                    <<use>>    │                     │
                                                               └─────────────────────┘
```



####MultiCorealtor:


The MultiCorealtor aggregates multiple corelators while implementing the correlator interface.
The multiCorelator stores a list of correlators. It Matches resources to templates by iterating over the list of corelators and for each subcorealeator attempts to find a match for the requested resource.
In case a match is found for one of the corelators, it will be returned without any errors.
If no match is found a joined error including all sub corealtors errors will be returned.


####MetricsCorelatorDecorator:

Wraps a single correlator, And collects metrics about the correlation. The metrics can be later retrieved and then can be used to create a summary output. The MetricsCorelatorDecorator gathers metrics on which resource templates that have been matched and with cluster CRs were not matched.

####ExactMatchCorelator:

Matches templates by exact match between a predefined config including pairs of Resource names and their equivalent template.The exact behavior of this corelator is described in Correlation by manual matches section.

####GroupCorelator

The group corelator implements the correlation behavior explained in  Correlation by group of fields (apiVersion, kind, namespace and name). The correlation behavior in this version is: “Each CR will be correlated to a template with an exact match in the largest number of fields from this group:  apiVersion, kind, namespace, name.”
The group corelator is more generic, and it gets on creation a list of fields that will be used for matching templates. In this version the group of fields are fixed:  apiVersion, kind, namespace, name. But it can be changed in the future to allow more flexibility in group correlating.


## Alternatives

### kubectl/oc diff
The existing kubectl/oc diff works well for validation of a CR (or set of CRs) on a cluster against
a known valid configuration. This tool does a good job of suppressing diffs in known managed fields
(eg metadata, status, etc), however it is lacking in several critical features for the use cases in
this enhancement:
* Suppression of expected user variations
* Handling of one-to-many matches
* Comparison of two offline files

### Command line utilities
Another option is the builtin diff command:
diff -t -y -w <(yq 'sort_keys(..)' /path/to/reference/config/cr) <(yq 'sort_keys(..)' /path/to/input/cr )
The command works well on Comparison of two offline files but doesn't handle one-to-many matches and does not supress known managed fields and expected user variations.
