package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"

	pb "github.com/rohitsakala/lpaas/api/gen/lpaas/v1alpha1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func main() {
	Execute()
}

// NewLpaasClient establishes an mTLS gRPC connection using global flags.
func NewLpaasClient() (*grpc.ClientConn, pb.LpaasClient, error) {
	// Load client certificate
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, nil, fmt.Errorf("failed loading client cert/key: %w", err)
	}

	// Load CA
	caPEM, err := os.ReadFile(caFile)
	if err != nil {
		return nil, nil, fmt.Errorf("failed reading CA file: %w", err)
	}
	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(caPEM) {
		return nil, nil, fmt.Errorf("failed adding CA certificate to pool")
	}

	serverName := "localhost"
	if host, _, err := net.SplitHostPort(serverAddr); err == nil && host != "" {
		serverName = host
	}

	// Build TLS config
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      certPool,
		ServerName:   serverName,
		NextProtos:   []string{"h2"},
	}

	conn, err := grpc.NewClient(
		serverAddr,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to %s: %w", serverAddr, err)
	}

	return conn, pb.NewLpaasClient(conn), nil
}
