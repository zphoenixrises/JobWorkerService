package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	api "github.com/zphoenixrises/JobWorkerService/api/proto"
)

type JobWorkerClient struct {
	conn   *grpc.ClientConn
	client api.JobWorkerClient
}

func NewJobWorkerClient() (*JobWorkerClient, error) {
	serverAddr := os.Getenv("SERVER_ADDRESS")
	if serverAddr == "" {
		return nil, fmt.Errorf("SERVER_ADDRESS environment variable is not set")
	}

	certFile := os.Getenv("CLIENT_CERT_FILE")
	keyFile := os.Getenv("CLIENT_KEY_FILE")
	caFile := os.Getenv("CA_CERT_FILE")

	if certFile == "" || keyFile == "" || caFile == "" {
		return nil, fmt.Errorf("CLIENT_CERT_FILE, CLIENT_KEY_FILE, and CA_CERT_FILE environment variables must be set")
	}

	// Load client cert
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load client cert: %v", err)
	}

	// Load CA cert
	caCert, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA cert: %v", err)
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to append CA cert")
	}

	// Create TLS config
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      certPool,
		MinVersion:   tls.VersionTLS13,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
		},
		CurvePreferences: []tls.CurveID{tls.CurveP256},
	}

	creds := credentials.NewTLS(tlsConfig)

	conn, err := grpc.NewClient(
		serverAddr,
		grpc.WithTransportCredentials(creds),
	)

	if err != nil {
		return nil, err
	}

	client := api.NewJobWorkerClient(conn)

	return &JobWorkerClient{
		conn:   conn,
		client: client,
	}, nil
}

func (c *JobWorkerClient) Close() error {
	return c.conn.Close()
}

func (c *JobWorkerClient) StartJob(ctx context.Context, command string, args []string, limits *api.ResourceLimits) (*api.StartJobResponse, error) {
	req := &api.StartJobRequest{
		Command:        command,
		Args:           args,
		ResourceLimits: limits,
	}
	return c.client.StartJob(ctx, req)
}

func (c *JobWorkerClient) StopJob(ctx context.Context, jobID string) (*api.StopJobResponse, error) {
	req := &api.JobId{
		Id: jobID,
	}
	return c.client.StopJob(ctx, req)
}

func (c *JobWorkerClient) GetJobStatus(ctx context.Context, jobID string) (*api.JobStatusResponse, error) {
	req := &api.JobId{
		Id: jobID,
	}
	return c.client.GetJobStatus(ctx, req)
}

func (c *JobWorkerClient) GetJobOutput(ctx context.Context, jobID string) (api.JobWorker_GetJobOutputClient, error) {
	req := &api.JobOutputRequest{
		Id: jobID,
	}
	return c.client.GetJobOutput(ctx, req)
}
