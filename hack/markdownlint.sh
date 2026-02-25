#!/bin/bash -ex
# Following example of: https://github.com/openshift/enhancements/blob/master/hack/markdownlint.sh

# trap errors, including the exit code from the command failed
trap 'handle_exit $?' EXIT

function handle_exit {
    # If the exit code we were given indicates an error, suggest that
    # the author run the linter locally.
    if [ "$1" != "0" ]; then
        cat - <<EOF

To run the linter on a Linux system with podman, run "make markdownlint"
after committing your changes locally.

EOF
    fi
}

markdownlint-cli2 '**/*.md' !'vendor/**/*.md'
