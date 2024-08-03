# Job Worker Service

This project implements a Job Worker Service that allows running and managing arbitrary Linux processes with fine-grained resource control and real-time output streaming.

## Prerequisites

- Go 1.22.5 or later
- Docker and Docker Compose
- OpenSSL (for certificate generation)

## Getting Started

1. Clone the repository:
   ```
   git clone https://github.com/zphoenixrises/JobWorkerService.git
   cd JobWorkerService
   ```

2. Install dependencies:
   ```
   make deps
   ```

## Generating Certificates

Before running the server or client, you need to generate certificates for mTLS:

```
make certs
```

This will create the necessary certificates in the `certs` directory.

## Running Tests

To run the tests in a Docker container:

```
make test
```

This command builds a test Docker image and runs the tests in a privileged container.

## Running the Server

To build and run the server in a Docker container:

```
make run-server
```

This command builds the server binary, creates a Docker image, and starts the server in a privileged container using Docker Compose.

To stop the server:

```
make stop-server
```

## Using the CLI

First, build the CLI client:

```
make build-client
```
Before running the CLI, you need to export the following into your shell environment:

export SERVER_ADDRESS=localhost:50051 && export CLIENT_CERT_FILE=./certs/client1.crt && export CLIENT_KEY_FILE=./certs/client1.key && export CA_CERT_FILE=./certs/ca.crt

Now you can use the CLI to interact with the server. Here are some example commands:

1. Start a job that prints "start", "middle" and "end":
   ```
   ./build/jobworker-client start .5 1048576 1048576 sh -c "echo 'start'; sleep 20; echo 'middle'; sleep 20; echo 'end'"
   ```
   This will return a job ID.

2. Stream the output of a job:
   ```
   ./build/jobworker-client stream <job-id>
   ```

3. Get the status of a job:
   ```
   ./build/jobworker-client status <job-id>
   ```

4. Stop a job:
   ```
   ./build/jobworker-client stop <job-id>
   ```

The command in 1. will pass. Here is an example of one that will fail:
```
 ./build/jobworker-client start .5 1024 1024 sh -c "echo 'start'; sleep 20; echo 'middle'; sleep 20; echo 'end'"
```

## Additional CLI Commands

- To see all available commands:
  ```
  ./build/jobworker-client
  ```

- To see help for a specific command:
  ```
  ./build/jobworker-client <command> 
  ```

## Development

- To format the code:
  ```
  make fmt
  ```

- To run the linter:
  ```
  make lint
  ```

- To regenerate protobuf files (if you've made changes to the .proto files):
  ```
  make proto
  ```

## Cleaning Up

To clean up build artifacts and Docker resources:

```
make clean
```

## Notes

- The server runs in a privileged Docker container to allow management of cgroups and other system resources.
- mTLS is used for secure communication between the client and server.
- Make sure the necessary environment variables are set when running the client (SERVER_ADDRESS, CLIENT_CERT_FILE, CLIENT_KEY_FILE, CA_CERT_FILE).

## Troubleshooting

If you encounter any issues:

1. Ensure all prerequisites are installed and up to date.
2. Check that the server is running and accessible.
3. Verify that the certificates have been generated correctly.
4. Make sure the correct environment variables are set for the client.

If problems persist, please open an issue in the GitHub repository.