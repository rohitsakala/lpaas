package main

import (
	"context"
	"fmt"

	pb "github.com/rohitsakala/lpaas/api/gen/lpaas/v1alpha1"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status <job-id>",
	Short: "Get the current status of a job",
	Args:  cobra.ExactArgs(1),

	RunE: func(cmd *cobra.Command, args []string) error {
		jobID := args[0]
		conn, client, err := NewLpaasClient()
		if err != nil {
			return err
		}
		defer conn.Close()

		resp, err := client.GetStatus(context.Background(), &pb.JobRequest{Id: jobID})
		if err != nil {
			return fmt.Errorf("failed to get status: %w", err)
		}

		fmt.Printf("Job %s:\n", resp.Id)
		fmt.Printf("  Status: %s\n", resp.Status)

		if resp.ExitCode != nil {
			fmt.Printf("  ExitCode: %d\n", *resp.ExitCode)
		}

		if resp.Error != nil && *resp.Error != "" {
			fmt.Printf("  Error: %s\n", *resp.Error)
		}

		return nil
	},
}

func init() {
	RootCmd.AddCommand(statusCmd)
}
