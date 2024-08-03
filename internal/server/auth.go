package server

import (
	"context"
	"fmt"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

func extractUsernameFromCert(ctx context.Context) (string, error) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return "", fmt.Errorf("no peer found")
	}

	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return "", fmt.Errorf("unexpected peer transport credentials")
	}

	if len(tlsInfo.State.VerifiedChains) == 0 || len(tlsInfo.State.VerifiedChains[0]) == 0 {
		return "", fmt.Errorf("could not verify peer certificate")
	}

	cert := tlsInfo.State.VerifiedChains[0][0]
	username := ""

	for _, ext := range cert.Extensions {
		if ext.Id.Equal([]int{1, 2, 3, 4, 5, 6, 7, 8}) { // OID for custom extension
			username = string(ext.Value)
			break
		}
	}

	if username == "" {
		return "", fmt.Errorf("no username found in certificate")
	}

	return username, nil
}
