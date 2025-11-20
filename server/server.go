package main

import (
	"crypto/tls"
	"crypto/x509"
	"log"
	"net"
	"os"

	lpaasv1alpha1 "github.com/rohitsakala/lpaas/api/gen/lpaas/v1alpha1"
	"github.com/rohitsakala/lpaas/pkg/server"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var (
	certFile = "certs/server.crt"
	keyFile  = "certs/server.key"
	caFile   = "certs/ca.crt"
	addr     = ":8443"
)

func main() {
	// Load server keypair
	serverCert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		log.Fatalf("failed loading server keypair: %v", err)
	}

	// Load CA for client authentication
	caPEM, err := os.ReadFile(caFile)
	if err != nil {
		log.Fatalf("failed reading CA file: %v", err)
	}
	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(caPEM) {
		log.Fatalf("failed to add CA certificate to pool")
	}

	// TLS configuration for gRPC mTLS
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientCAs:    certPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS13,
		NextProtos:   []string{"h2"},
	}

	// gRPC server with TLS
	creds := credentials.NewTLS(tlsCfg)
	grpcServer := grpc.NewServer(grpc.Creds(creds))

	// Register your LPaaS service
	srv := server.NewServer()
	lpaasv1alpha1.RegisterLpaasServer(grpcServer, srv)

	// Listen on TCP
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", addr, err)
	}

	log.Printf("gRPC worker listening on %s (mTLS required)", addr)

	if err := grpcServer.Serve(ln); err != nil {
		log.Fatalf("grpc Serve error: %v", err)
	}
}
