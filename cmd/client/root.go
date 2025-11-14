package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	serverAddr string
	caFile     string
	certFile   string
	keyFile    string
)

var RootCmd = &cobra.Command{
	Use:           "lpaas",
	Short:         "LPaaS CLI — Linux job processes via gRPC",
	Long:          "A CLI client to start, stop, and manage Linux processes via the LPaaS gRPC worker service.",
	SilenceErrors: true,
	SilenceUsage:  true,
	CompletionOptions: cobra.CompletionOptions{
		DisableDefaultCmd: true,
	},
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	flags := RootCmd.PersistentFlags()

	flags.StringVar(&serverAddr, "addr", "localhost:8443", "Server gRPC address")
	flags.StringVar(&caFile, "ca", "certs/ca.crt", "CA certificate")
	flags.StringVar(&certFile, "cert", "certs/client.crt", "Client certificate")
	flags.StringVar(&keyFile, "key", "certs/client.key", "Client private key")
}
