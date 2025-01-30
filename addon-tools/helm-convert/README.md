# helm-convertor

`helm converter` is a tool allowing conversion of kube-compare reference configs in to valid helm charts.
The converted helm charts then can be used to create a group of CRs that is a valid configuration by rendering the helm chart
with user defined values.
The helm chart that is created supports easily creation of Many CRs from the same template.

## Build

Build the code locally:

```shell
make build-helm-converter
```

## Output

Helm Convertor outputs a helm chart including templates and an example values.yaml file. the values.yaml file should include all the variables
that are needed in order to build/recreate the CRs from the reference templates.
the values.yaml file is a yaml file where its keys are the names of the templates in the reference
(the name includes the path from the reference root joined with "_" separator and without the template file ending).
the value for each template includes a slice. Each element in the slice represents the values needed to create one instance of a CR from the template.
each element in the slice includes the required values to create the CR. The values.yaml can of course include the whole CR  
but the best use case is to include the minimal number of values in order to build the template. This tool adds some features
that can help in finding the minimum values needed in order to build the CRs from the templates.
It's important to mention that the user is in charge of completing the values.yaml, because it includes his own preferences/configuration.

example:

```yaml
DirName_TemplateName:
- apiVersion: v1
  metadata:
    name: defaultName  
    namespace: default
sa: # template in same dir as metadata.yaml 
- apiVersion: v2
  metadata:
    name: example
DirName_DirName2_secret:
- metadata:   # values for first CR that will be created from  DirName/DirName2/secret.yaml template 
    name: name
- metadata:  # values for second CR that will be created from  DirName/DirName2/secret.yaml template 
    name: name2
```

helm convertor creates a helm chart where each template in the kube-compare formatted reference includes a corresponding template in the helm chart.
The helm convertor wraps each template in an additional `range` defined in go templating syntax, the range is defined to loop
over the CRs values as defined in the chart values. This results in a creation of a CR for each element in the corresponding
slice in the values.yaml.

## Easy Creation of values.yaml

creating a values.yaml from scratch may be very frustrating to do without any automation.
This can be complicated for references including multiple templates, because it requires going over all the templates,
and gathering a list of which values are required to be able to build the CR.

To address this problem the Tool creates a values.yaml file structure that contains values that are required in order
to build the CRs. Then the user only needs to fill in the values.yaml file with the default values without needing to
think about which values are needed and without needing to build the structure.
The list of values that are needed are collected by a traversal on the static form of template.
Due to the big amount of possibles in go templating syntax some cases where values are used can be not detected.
This version covers all the basic use-cases, More complex cases can be added by demand. Additionally, different
approaches to this issue can be added.  
The initial implementation of the values gathering is a great place to start with, and for complex cases users may will
need to so some minor modifications. This is also a one time thing per reference, for references that have already been converted
the previous values.yaml can be used as a base for the values.yaml file. Most of the work is in the initial conversion.

example generated values.yaml structure (does not include the default values):

```yaml
#gnerated from: pkg/compare/testdata/AllRequiredTemplatesExistAndThereAreNoDiffs/reference/metadata.yaml
deploymentMetrics:
- spec:
    template:
      spec: {}
role:
- apiVersion: {}
  metadata:
    name: {}  # user needs to replace {} with default value (for many {} in example)
    namespace: {}
sa:
- apiVersion: {}
  metadata:
    name: {}
secret:
- data: {}
  metadata:
    name: {}
service:
- metadata:
    labels: {}
    name: {}
  spec:
    ports:
      "0":
        port: {}
        targetPort: {}
    selector:
      k8s-app: {}
```

### Capturegroup default substitution

The helm-convert tool supports a mechanism to substitute capturegroup default
values if required.  If the defaults.yaml contains a section called
`captureGroup_defaults`, and the YAML in question contains one or more
captureGroups using either the `regex` or `capturegroup` inlineDiff mechanism,
all capturegroups with a default in the `captureGroup_defaults` section will be
replaced by the default value when converting the reference template to helm
chart template.

For example, if you have a CR like this:

```yaml
apiVersion: v1
Kind: Foo
spec:
  value: |-
    Something with (?<blee>.*) in it,
    And another capturegroup (?<bar>.*) with no default.
```

With this in the values.yaml:

```yaml
Foo:
- captureGroup_defaults:
    blee: 42
```

The resulting Helm template will look like this:

```yaml
apiVersion: v1
Kind: Foo
spec:
  value: |-
    Something with 42 in it,
    And another capturegroup (?<bar>.*) with no default.
```

## Auto Extracting of default values from Existing CRs

another feature that can help in initial building of values.yaml files is extracting default values from existing CRs,
The command gets a path to a directory that should include CRs that there name is the same as the corresponding template.
For each template that includes a CR with the same name in the defaults directory the tool will extract the defaults values
from the corresponding CR.

## Run

Basic usage:

```shell
# create helm chart
helm-convert -r ./reference/metadata.yaml -n ChartDir 
# Render templates, resulting in creation of default CRs that follow the reference 
helm template renderedref ChartDir/ChartDir/ --output-dir renderedRefDir
```

Auto Extracting of default values from Existing CRs:

```shell
helm-convert -r ./reference/metadata.yaml -n ChartDir -d ExstisingCRsDir
helm template renderedref ChartDir/ChartDir/ --output-dir renderedRefDir 
```

### Updating templates after update of reference

when updating kube-compare references, its recommended when recreating  the helm chart to use the previous values.yaml file
as a base for the new values.yaml file to do so run as in the following example:

```shell
helm-convert -r ./reference/metadata.yaml -n ChartDir -v previousValues.yaml
helm template renderedref ChartDir/ChartDir/ --output-dir renderedRefDir 
```
