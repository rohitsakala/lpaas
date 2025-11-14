package main

import (
	"context"
	"fmt"

	pb "github.com/rohitsakala/lpaas/api/gen/lpaas/v1alpha1"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start [--] <command> [args...]",
	Short: "Start a new job on the LPaaS worker",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, client, err := NewLpaasClient()
		if err != nil {
			return err
		}
		defer conn.Close()

		resp, err := client.StartJob(context.Background(), &pb.StartJobRequest{
			Command: args[0],
			Args:    args[1:],
		})
		if err != nil {
			return fmt.Errorf("failed to start job: %w", err)
		}

		fmt.Printf("Job started with ID: %s\n", resp.Id)
		return nil
	},
}

func init() {
	RootCmd.AddCommand(startCmd)
}
