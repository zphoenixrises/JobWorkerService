# Go parameters
GOCMD      := go
GOCLEAN    := $(GOCMD) clean
GOGET      := $(GOCMD) get
GOMOD      := $(GOCMD) mod
GOFMT      := $(GOCMD) fmt

# Docker parameters
TEST_DOCKER_IMAGE := jobworker-test
TEST_DOCKERFILE   := Dockerfile.test

# Targets
all: test 

test:
	docker build -t $(TEST_DOCKER_IMAGE) -f $(TEST_DOCKERFILE) . && \
	docker run --privileged -it --rm -v /sys/fs/cgroup:/sys/fs/cgroup:rw $(TEST_DOCKER_IMAGE)

clean:
	$(GOCLEAN)
	rm -rf build

deps:
	$(GOGET) -v -d ./...
	$(GOMOD) tidy

fmt:
	$(GOFMT) ./...

lint:
	golangci-lint run

# Help target
help:
	@echo "Available targets:"
	@echo "  test         - Run tests"
	@echo "  clean        - Clean build artifacts"
	@echo "  deps         - Get dependencies and tidy go.mod"
	@echo "  fmt          - Format Go code"
	@echo "  lint         - Run linter"

.PHONY: test clean deps fmt lint help
