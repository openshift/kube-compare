

# Rationale

The “kubectl cluster-compare” command is capable of performing an intelligent diff between a reference configuration and
the specific configuration applied to a cluster. The comparison is capable of suppressing diffs of content which is
expected to be user variable, validating required and optional configuration, and ignoring known runtime variable
fields. With these fields suppressed the user is able to focus on the remaining diffs which are more
relevant/potentially impactful to the use case. With these capabilities a cluster administrator, solutions architect,
support engineers, and others can validate a cluster’s configuration against a baseline reference configuration.

# Concepts

## Optional vs required

## CR selection (correlation)

The "kubectl cluster-compare" tool works by iterating across the CRs found in the reference configuration and finding
CRs from the users configuration (live cluster or collection of CRs) which should be compared to the
reference. Typically the relevant user CRs have unique names or are contained in user defined namespaces. The tool
correlates reference CRs to user CRs as described in this section in order to account for the expected variatoins in
naming without requiring the user to explicitly map reference to cluster CR.

### Correlating CRs

`kubectl cluster-compare` must correlate CRs between reference and input configurations to perform the
comparisons. `kubectl cluster-compare` correlates CRs by using the apiVersion, kind, namespace and name fields of the
CRs to perform a nearest match correlation. Optionally the user may provide a manual override of the correlation to
identify a specific reference configuration CR to be used for a given user input CR. Manual matches are prioritized over
the automatic of correlation, meaning manual matches override matches by similar values in the specified group of
fields.

#### Correlation by manual matches

`kubectl cluster-compare` gets as input a diff config that contains an option to specify manual matches between cluster
resources and resource templates. The matches can be added to the config as pairs of apiVersion_kind_namespace_name:
<Template File Name>. For cluster scoped CRs that don't have a namespace the matches can be added as pairs of
apiVersion_kind_name: <Template File Name>.

#### Correlation by group of fields (apiVersion, kind, namespace and name)

When there is no manual match for a CR the command will try to match a template for the resource by looking at the
4-tuple: apiVersion, kind, namespace and name . The Correlation is based on which fields in the templates that are not
user-variable. Templates get matched to resources based on all the features from the 4-tuple that are declared fixed (
not user-variable) in the templates.

For example a template with a fixed namespace, kind, name and templated (user-variable) apiVersion will only be a
potential match by the kind-namespace-name criterion.

For each resource the group correlation will be done by the next logic:

1. Exact match of apiVersion-kind-namespace-name
    1. If single result in reference, comparison will be done
1. Exact Match in 3/4 fields from apiVersion, kind, namespace, name. ( meaning exact match in: kind-namespace-name or
   apiVersion-kind-name or apiVersion-kind-namespace)
    1. If single result in reference, comparison will be done
1. Exact Match in 2/4 fields from apiVersion, kind, namespace, name. ( meaning exact match in: kind-namespace or
   kind-name or apiVersion-kind)
    1. If single result in reference, comparison will be done
1. Match kind
    1. If single result in reference, comparison will be done
1. No match – comparison cannot be made and the file is flagged as unmatched.

We can phrase this logic in a more general form. Each CR will be correlated to a template with an exact match in the
largest number of fields from this group:  apiVersion, kind, namespace, name.

## How it works 
* eg how templates pull content into reference prior to compare

## Limits
## Single CR scope
This tool provides a context aware diff function. In some cases the reference may further provide validation of values
in the CRs. This validation operates within the scope of a the current CR. Any validation of values across multiple CRs
is out of scope.

## CR validation
This tool performs a diff of a CR against the reference. It does not have access to resources outside the scope of what
is available through the cluster's API. This places validation of the configuration against underlying platform
hardware, os configuratoin, etc (unless available through the api) out of scope.

# Example use cases

# Understanding the output

# Options and advanced usage

# Troubleshooting
