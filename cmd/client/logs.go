package main

import (
	"fmt"
	"io"
	"os"

	pb "github.com/rohitsakala/lpaas/api/gen/lpaas/v1alpha1"
	"github.com/spf13/cobra"
)

var logsCmd = &cobra.Command{
	Use:   "stream-logs <job-id>",
	Short: "Stream the output of a running or completed job",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		jobID := args[0]

		conn, client, err := NewLpaasClient()
		if err != nil {
			return err
		}
		defer conn.Close()

		stream, err := client.StreamOutput(cmd.Context(), &pb.StreamRequest{Id: jobID})
		if err != nil {
			return fmt.Errorf("stream start error: %w", err)
		}

		fmt.Printf("Streaming logs for job %s...\n", jobID)

		for {
			chunk, err := stream.Recv()
			if err == io.EOF {
				fmt.Println("\nStream ended.")
				return nil
			}
			if err != nil {
				return fmt.Errorf("stream recv error: %w", err)
			}

			_, writeErr := os.Stdout.Write(chunk.Data)
			if writeErr != nil {
				return fmt.Errorf("stdout write error: %w", writeErr)
			}
		}
	},
}

func init() {
	RootCmd.AddCommand(logsCmd)
}
