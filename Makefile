

.PHONY: build
build:
	go build ./cmd/kubectl-cluster_compare.go

.PHONY: test
test:
	go test --race ./pkg/*
