package main

import (
	"fmt"

	pb "github.com/rohitsakala/lpaas/api/gen/lpaas/v1alpha1"
	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop <job-id>",
	Short: "Stop a running job on the LPaaS worker",
	Args:  cobra.ExactArgs(1),

	RunE: func(cmd *cobra.Command, args []string) error {
		jobID := args[0]

		conn, client, err := NewLpaasClient()
		if err != nil {
			return err
		}
		defer conn.Close()

		_, err = client.StopJob(cmd.Context(), &pb.JobRequest{Id: jobID})
		if err != nil {
			return fmt.Errorf("failed to stop job: %w", err)
		}

		fmt.Printf("Job %s stopped successfully\n", jobID)
		return nil
	},
}

func init() {
	RootCmd.AddCommand(stopCmd)
}
