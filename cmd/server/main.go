package main

import (
	"log"
	"os"

	"github.com/zphoenixrises/JobWorkerService/internal/server"
)

func main() {
	serverCert := os.Getenv("SERVER_CERT")
	serverKey := os.Getenv("SERVER_KEY")
	caCert := os.Getenv("CA_CERT")

	if serverCert == "" || serverKey == "" || caCert == "" {
		log.Fatal("SERVER_CERT, SERVER_KEY, and CA_CERT environment variables must be set")
	}

	jobWorkerServer := server.NewJobWorkerServer()

	// Bind to all interfaces
	if err := jobWorkerServer.Run("0.0.0.0:50051", serverCert, serverKey, caCert); err != nil {
		log.Fatalf("Failed to run server: %v", err)
	}
}
