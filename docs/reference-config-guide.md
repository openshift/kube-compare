
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
Components and components are groups of CR templates.

Each component includes required templates and optional templates. Required templates are CRs that must be present in
the input configuration. They are reported in the summary as "missing content" in case there are no cluster CRs that
correlate with the required template. Optional CRs will not be reported in the summary as "missing content".

A component can also be flagged as optional, in this case if any of the
required CRs are included in the input configuration then all required CRs
in the group are expected to be included and any which are missing will be reported. If none of
the required CRs in the group are included then no report of "missing content" for the group will be generated.

In this version Parts only help with organization of the components into groups and don't have any affection on the diff
process.

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
