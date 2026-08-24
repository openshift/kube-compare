#!/bin/bash
sed -i 's/func TestHelperProcess(t \*testing.T) {/func TestHelperProcess(_ \*testing.T) {/' pkg/compare/container_test.go
sed -i 's/func TestCleanup(t \*testing.T) {/func TestCleanup(_ \*testing.T) {/' pkg/compare/container_test.go
sed -i 's/lookPath = func(cmd string) (string, error) {/lookPath = func(c string) (string, error) {/g' pkg/compare/container_test.go
sed -i 's/cmd == podman/c == podman/g' pkg/compare/container_test.go
sed -i 's/cmd == docker/c == docker/g' pkg/compare/container_test.go
