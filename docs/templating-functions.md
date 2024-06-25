# Templating functions

On top of golangs normal templating we provide some functions to make writitng references easier.
The (sprig functions)[http://masterminds.github.io/sprig/] are included along with some other functions that can be found in the (subpackages pkg.go.dev page)[https://pkg.go.dev/github.com/openshift/kube-compare/pkg/funcmap#pkg-functions]

If you want to custom functions you can define them as a templates and include them a paths under `templateFunctionFiles` at the root of your reference `metadata.yml`