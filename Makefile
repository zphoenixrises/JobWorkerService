# Go parameters
GOCMD      := go
GOBUILD    := $(GOCMD) build
GOCLEAN    := $(GOCMD) clean
GOGET      := $(GOCMD) get
GOMOD      := $(GOCMD) mod
GOFMT      := $(GOCMD) fmt

# Docker parameters
TEST_DOCKER_IMAGE := jobworker-test
TEST_DOCKERFILE   := Dockerfile.test
DOCKER_COMPOSE    := docker-compose

# Build parameters
SERVER_BINARY := build/jobworker-server
CLIENT_BINARY := build/jobworker-client

# Targets
all: test build

build: build-server build-client

build-server:
	$(GOBUILD) -o $(SERVER_BINARY) ./cmd/server

build-client:
	$(GOBUILD) -o $(CLIENT_BINARY) ./cmd/client

run-server: build-server
	$(DOCKER_COMPOSE) up --build

stop-server:
	$(DOCKER_COMPOSE) down

run-client: build-client
	./$(CLIENT_BINARY)

test:
    # docker is used so that we can run this in any OS
	docker build -t $(TEST_DOCKER_IMAGE) -f $(TEST_DOCKERFILE) . && \
	docker run --privileged -it --rm -v /sys/fs/cgroup:/sys/fs/cgroup:rw $(TEST_DOCKER_IMAGE)

clean:
	$(GOCLEAN)
	rm -rf build
	rm -f $(SERVER_BINARY) $(CLIENT_BINARY)
	$(DOCKER_COMPOSE) down -v --rmi all --remove-orphans

deps:
	$(GOGET) -v -d ./...
	$(GOMOD) tidy

fmt:
	$(GOFMT) ./...

lint:
	golangci-lint run

# Generate protobuf files
proto:
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		api/proto/jobworker.proto

# Generate certificates
certs:
	sh ./generate_certs.sh
	sh ./generate_client_certs.sh zeeshan client1
# Help target
help:
	@echo "Available targets:"
	@echo "  build        - Build both server and client"
	@echo "  build-server - Build the server binary"
	@echo "  build-client - Build the client binary"
	@echo "  run-server   - Build and run the server using Docker Compose"
	@echo "  stop-server  - Stop the server running in Docker Compose"
	@echo "  run-client   - Build and run the client locally"
	@echo "  test         - Run tests"
	@echo "  clean        - Clean build artifacts and Docker resources"
	@echo "  deps         - Get dependencies and tidy go.mod"
	@echo "  fmt          - Format Go code"
	@echo "  lint         - Run linter"
	@echo "  proto        - Generate protobuf files"
	@echo "  certs        - Generate certificates for mTLS"

.PHONY: all build build-server build-client run-server stop-server run-client test clean deps fmt lint proto certs help