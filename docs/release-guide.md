# Release Guide

The plugin uses GitHub releases to store binaries and release info. The release notes and binaries are created using
GoReleaser allowing to build the binary for different architectures and platforms. The configuration of GoReleaser
is located in .goreleaser.yml.

## Release instructions

1. In your local clone of the project tag the commit using git, the tags value should be the number of the release
   formatted as "v[major].[minor].[tiny]". for example "v1.0.0".
2. Run `make release-dry-run`, The release files will be created in the dist directory. make sure the release contains
   the expected changes. once you are satisfied continue to the next step.
3. Set GITHUB_TOKEN environment variable to be your GitHub api token, the minimum permissions needed are
   `packages:write`. (run `export GITHUB_TOKEN="<YOUR_TOKEN>"`).
4. Run `make release`, the command should upload the release to GitHub.

The release notes can be edited manually at any time if needed [here](https://github.com/openshift/kube-compare/releases).
