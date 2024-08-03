package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"net"
	"os"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	api "github.com/zphoenixrises/JobWorkerService/api/proto"
	"github.com/zphoenixrises/JobWorkerService/pkg/jobworker"
)

type UserJobs struct {
	jobs map[string]*jobworker.Executor
	mu   sync.RWMutex
}

type JobWorkerServer struct {
	api.UnimplementedJobWorkerServer
	users     map[string]*UserJobs
	usersLock sync.RWMutex
}

func NewJobWorkerServer() *JobWorkerServer {
	return &JobWorkerServer{
		users: make(map[string]*UserJobs),
	}
}

func (s *JobWorkerServer) Run(address, certFile, keyFile, caFile string) error {
	// Load server's certificate and key
	serverCert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return fmt.Errorf("failed to load server certificate and key: %v", err)
	}

	// Load CA certificate
	caCert, err := os.ReadFile(caFile)
	if err != nil {
		return fmt.Errorf("failed to read CA certificate: %v", err)
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(caCert) {
		return fmt.Errorf("failed to add CA certificate to pool")
	}

	// Create TLS configuration
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    certPool,
		MinVersion:   tls.VersionTLS13,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
		},
		CurvePreferences: []tls.CurveID{tls.CurveP256},
	}

	// Create gRPC server with TLS credentials
	grpcServer := grpc.NewServer(
		grpc.Creds(credentials.NewTLS(tlsConfig)),
		grpc.UnaryInterceptor(s.unaryInterceptor),
		grpc.StreamInterceptor(s.streamInterceptor),
	)

	// Register the JobWorker service
	api.RegisterJobWorkerServer(grpcServer, s)

	// Start listening
	lis, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
	}

	log.Printf("Starting gRPC server with mTLS on %s\n", address)
	return grpcServer.Serve(lis)
}

func (s *JobWorkerServer) unaryInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	username, err := s.authenticateAndAuthorize(ctx, info.FullMethod)
	if err != nil {
		return nil, err
	}

	newCtx := context.WithValue(ctx, "username", username)
	return handler(newCtx, req)
}

func (s *JobWorkerServer) streamInterceptor(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	username, err := s.authenticateAndAuthorize(ss.Context(), info.FullMethod)
	if err != nil {
		return err
	}

	newCtx := context.WithValue(ss.Context(), "username", username)
	wrappedStream := newWrappedServerStream(ss, newCtx)
	return handler(srv, wrappedStream)
}

func (s *JobWorkerServer) authenticateAndAuthorize(ctx context.Context, method string) (string, error) {
	username, err := extractUsernameFromCert(ctx)
	if err != nil {
		return "", status.Errorf(codes.Unauthenticated, "authentication failed: %v", err)
	}

	return username, nil
}

func (s *JobWorkerServer) getUserJobs(username string) *UserJobs {
	s.usersLock.RLock()
	userJobs, ok := s.users[username]
	s.usersLock.RUnlock()

	if !ok {
		s.usersLock.Lock()
		userJobs = &UserJobs{
			jobs: make(map[string]*jobworker.Executor),
		}
		s.users[username] = userJobs
		s.usersLock.Unlock()
	}

	return userJobs
}

type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func newWrappedServerStream(ss grpc.ServerStream, ctx context.Context) *wrappedServerStream {
	return &wrappedServerStream{ServerStream: ss, ctx: ctx}
}

func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}
