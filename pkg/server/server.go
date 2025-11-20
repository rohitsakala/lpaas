package server

import (
	"context"
	"fmt"
	"io"
	"sync"

	lpaasv1alpha1 "github.com/rohitsakala/lpaas/api/gen/lpaas/v1alpha1"
	"github.com/rohitsakala/lpaas/pkg/linuxjobs"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// extractOwnerFromTLS returns the client's identity from the mTLS certificate
// by reading the Common Name (CN) of the first peer certificate.
func extractOwnerFromTLS(ctx context.Context) (string, error) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return "", fmt.Errorf("no peer info in context")
	}
	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return "", fmt.Errorf("no TLS info available")
	}
	state := tlsInfo.State
	if len(state.PeerCertificates) == 0 {
		return "", fmt.Errorf("no peer certificate found")
	}
	return state.PeerCertificates[0].Subject.CommonName, nil
}

// Server implements the Lpaas gRPC service and manages a JobManager per owner.
type Server struct {
	lpaasv1alpha1.UnimplementedLpaasServer
	mu       sync.RWMutex
	managers map[string]*linuxjobs.JobManager
}

// NewServer creates a new Server instance with an empty manager map.
func NewServer() *Server {
	return &Server{
		managers: make(map[string]*linuxjobs.JobManager),
	}
}

// getOrCreateManager returns the JobManager for the given owner, creating one
// if it does not already exist.
func (s *Server) getOrCreateManager(owner string) (*linuxjobs.JobManager, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if mgr, ok := s.managers[owner]; ok {
		return mgr, nil
	}

	mgr, err := linuxjobs.NewJobManager()
	if err != nil {
		return nil, fmt.Errorf("failed to create JobManager for owner %s: %v", owner, err)
	}

	s.managers[owner] = mgr
	return mgr, nil
}

// managerForOwner returns the JobManager for an owner if it exists.
func (s *Server) managerForOwner(owner string) (*linuxjobs.JobManager, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	mgr, ok := s.managers[owner]
	return mgr, ok
}

// StartJob starts a new job for the authenticated owner.
func (s *Server) StartJob(ctx context.Context, req *lpaasv1alpha1.StartJobRequest) (*lpaasv1alpha1.StartJobResponse, error) {
	owner, err := extractOwnerFromTLS(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "failed to extract identity: %v", err)
	}

	mgr, err := s.getOrCreateManager(owner)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get or create job manager: %v", err)
	}

	id, err := mgr.StartJob(req.Command, req.Args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to start job: %v", err)
	}

	return &lpaasv1alpha1.StartJobResponse{Id: id}, nil
}

// StopJob stops a running job owned by the authenticated client.
func (s *Server) StopJob(ctx context.Context, req *lpaasv1alpha1.JobRequest) (*lpaasv1alpha1.StopJobResponse, error) {
	owner, err := extractOwnerFromTLS(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "failed to extract identity: %v", err)
	}

	mgr, ok := s.managerForOwner(owner)
	if !ok {
		return nil, status.Errorf(codes.NotFound, "jobManager for owner %s not found", owner)
	}

	if !mgr.JobExists(req.Id) {
		return nil, status.Errorf(codes.NotFound, "job %s not found", req.Id)
	}

	if err := mgr.StopJob(req.Id); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to stop job %s: %v", req.Id, err)
	}

	return &lpaasv1alpha1.StopJobResponse{}, nil
}

// GetStatus returns the status of a job owned by the authenticated client.
func (s *Server) GetStatus(ctx context.Context, req *lpaasv1alpha1.JobRequest) (*lpaasv1alpha1.StatusJobResponse, error) {
	owner, err := extractOwnerFromTLS(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "failed to extract identity: %v", err)
	}

	mgr, ok := s.managerForOwner(owner)
	if !ok {
		return nil, status.Errorf(codes.NotFound, "jobManager for owner %s not found", owner)
	}

	if !mgr.JobExists(req.Id) {
		return nil, status.Errorf(codes.NotFound, "job %s not found", req.Id)
	}

	statusVal, code, jobErr := mgr.Status(req.Id)

	resp := &lpaasv1alpha1.StatusJobResponse{
		Id:     req.Id,
		Status: statusVal,
	}
	if code != nil {
		resp.ExitCode = code
	}
	if jobErr != nil {
		msg := jobErr.Error()
		resp.Error = &msg
	}
	return resp, nil
}

// StreamOutput streams the stdout and stderr of a job owned by the
// authenticated client.
func (s *Server) StreamOutput(req *lpaasv1alpha1.StreamRequest, stream lpaasv1alpha1.Lpaas_StreamOutputServer) error {
	owner, err := extractOwnerFromTLS(stream.Context())
	if err != nil {
		return status.Errorf(codes.Unauthenticated, "failed to extract identity: %v", err)
	}

	mgr, ok := s.managerForOwner(owner)
	if !ok {
		return status.Errorf(codes.NotFound, "jobManager for owner %s not found", owner)
	}

	if !mgr.JobExists(req.Id) {
		return status.Errorf(codes.NotFound, "job %s not found", req.Id)
	}

	reader, err := mgr.StreamJob(req.Id)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to stream job %s: %v", req.Id, err)
	}
	defer reader.Close()

	buf := make([]byte, 4096)
	for {
		n, readErr := reader.Read(buf)
		if n > 0 {
			if sendErr := stream.Send(&lpaasv1alpha1.StreamChunk{Data: buf[:n]}); sendErr != nil {
				return status.Errorf(codes.Unavailable, "failed to send stream chunk: %v", sendErr)
			}
		}

		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return status.Errorf(codes.Internal, "stream error for job %s: %v", req.Id, readErr)
		}
	}
}
