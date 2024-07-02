# kube-compare

`kube-compare` allows the user to compare an individual manifest or entire running cluster to a reference and find the differences. Expected differences are ignored as are cluster-managed fields, highlighting only differences that are of interest.

`kube-compare` is intended for administrators, architects, support engineers, and others to quickly check that a configuration is as-expected. For a more detailed description of the purpose and approach of the tool please read the [proposal document](docs/proposal.md).

## Install

TODO make this a one-liner to setup the plugin/container

```shell
make build
```

## Run

A reference configuration is required in order to run. A reference configuration is a directory containing a [`metadata.yaml`](#metadatayaml) and one or more templates.

Specify the directory containing a reference with `-r`, and one or more manifests to compare to it with `-f`.

```shell
./kubectl-cluster_compare -r pkg/compare/testdata/YAMLOutput/reference/ -f pkg/compare/testdata/YAMLOutput/resources/d1.yaml
```

To compare all manifests in a directory use `-R`

```shell
./kubectl-cluster_compare -r pkg/compare/testdata/YAMLOutput/reference/ -f pkg/compare/testdata/YAMLOutput/resources/ -R
```

## Output

The tool outputs a diff for each comparison made, and a final summary.

Each comparison is surrounded by a line of `*`. The comparison identifies the cluster manifest and reference file being compared and a `diff`:

```diff
**********************************

Cluster CR: apps/v1_Deployment_kubernetes-dashboard_kubernetes-dashboard
Reference File: deploymentDashboard.yaml
Diff Output: diff -u -N /tmp/MERGED-4218954955/apps-v1_deployment_kubernetes-dashboard_kubernetes-dashboard /tmp/LIVE-168878603/apps-v1_deployment_kubernetes-dashboard_kubernetes-dashboard
--- /tmp/MERGED-4218954955/apps-v1_deployment_kubernetes-dashboard_kubernetes-dashboard 2024-07-02 09:18:04.314476186 -0400
+++ /tmp/LIVE-168878603/apps-v1_deployment_kubernetes-dashboard_kubernetes-dashboard    2024-07-02 09:18:04.314476186 -0400
@@ -14,7 +14,7 @@
   template:
     metadata:
       labels:
-        k8s-app: kubernetes-dashboard
+        k8s-app: kubernetes-dashboard-diff
     spec:
       containers:
       - args:

**********************************
```

The output ends with a summary of all comparisons made, which lists differences, and highlights if references were not found for any cluster manifests.

```shell
Summary
CRs with diffs: 1
CRs in reference missing from the cluster: 1
ExamplePart:
  Dashboard:
  - deploymentMetrics.yaml
No CRs are unmatched to reference CRs
```

## Metadata.yaml

At the basic level, `metadata.yaml` lays out a reference configuration in `Part`s, each containing `Component`s and defines the templates and comparison rules.

The following example describes an one-part guestbook app, with redis and frontend components. The templates it references are stored in the same directory as `metadata.yaml`.

```yaml
Parts:
  - name: guestbook
    Components:
      - name: redis
        type: Required # mark the Component as "Required" or "Optional"
        requiredTemplates: # absence from cluster manifests is considered a diff
          - path: redis-master-deployment.yaml
          - path: redis-master-service.yaml
        optionalTemplates: # will not be reported if missing from cluster manifests
          - path: redis-replica-deployment.yaml
          - path: redis-replica-service.yaml
      - name: frontend
        type: Required
        requiredTemplates:
          - path: frontend-deployment.yaml
          - path: frontend-service.yaml
fieldsToOmit:
  - - "metadata"
    - "labels"
    - "k8s-app" # remove metadata.labels.k8s-app before diff
```

`metadata.yaml` supports several other advanced behaviours:

* Declaring predefined fragments using `templateFunctionFiles` that can be used in multiple resource templates.
* Globally ignoring specific fields by yaml path with `fieldsToOmit`.
* Ignoring, per template, fields that are not defined in the reference template with `ignore-unspecified-fields`.

See the included [test cases](pkg/compare/testdata/) for more examples of reference configs. For a complete explanation of `metadata.yaml` please see [Building a Reference Config](docs/reference-config-guide.md).

## Further Resources

[User Guide](docs/user-guide.md)

[Building a Reference Config](docs/reference-config-guide.md)

[Developer Intro](docs/dev.md)
