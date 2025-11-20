package test

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"testing"
	"time"

	lpaasv1alpha1 "github.com/rohitsakala/lpaas/api/gen/lpaas/v1alpha1"
	"github.com/rohitsakala/lpaas/pkg/server"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

func ctxWithCN(cn string) context.Context {
	cert := &x509.Certificate{
		Subject: pkix.Name{CommonName: cn},
	}
	info := credentials.TLSInfo{
		State: tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{cert},
		},
	}
	p := &peer.Peer{AuthInfo: info}
	return peer.NewContext(context.Background(), p)
}

// Fake stream for StreamOutput
type fakeStream struct {
	lpaasv1alpha1.Lpaas_StreamOutputServer
	ctx context.Context
	buf bytes.Buffer
}

func (f *fakeStream) Context() context.Context { return f.ctx }

func (f *fakeStream) Send(c *lpaasv1alpha1.StreamChunk) error {
	if len(c.GetData()) == 0 {
		return nil
	}
	f.buf.Write(c.GetData())
	return nil
}

func (f *fakeStream) all() string {
	return f.buf.String()
}

// Test Authentication
func TestAuthentication_Success(t *testing.T) {
	t.Parallel()

	s := server.NewServer()
	ctx := ctxWithCN("rohit")

	resp, err := s.StartJob(ctx, &lpaasv1alpha1.StartJobRequest{
		Command: "bash",
		Args:    []string{"-c", "echo hi"},
	})

	require.NoError(t, err)
	require.NotEmpty(t, resp.Id)
}

// Test Authentication failure without TLS
func TestAuthentication_FailsWithoutTLS(t *testing.T) {
	t.Parallel()

	s := server.NewServer()
	ctx := context.Background()

	_, err := s.StartJob(ctx, &lpaasv1alpha1.StartJobRequest{
		Command: "bash",
		Args:    []string{"-c", "echo hi"},
	})

	require.Error(t, err)
	require.Equal(t, codes.Unauthenticated, status.Code(err))
}

// Test Authorization isolation between users
func TestAuthorization_Isolation(t *testing.T) {
	t.Parallel()

	s := server.NewServer()

	ctxRohit := ctxWithCN("rohit")
	ctxJyoshna := ctxWithCN("jyoshna")

	resp, err := s.StartJob(ctxRohit, &lpaasv1alpha1.StartJobRequest{
		Command: "bash",
		Args:    []string{"-c", "sleep 1"},
	})
	require.NoError(t, err)

	_, err = s.StopJob(ctxJyoshna, &lpaasv1alpha1.JobRequest{Id: resp.Id})

	require.Error(t, err)
	require.Equal(t, codes.NotFound, status.Code(err))
}

// Test Start Status Stream
func TestServer_Start_Status_Stream(t *testing.T) {
	t.Parallel()

	s := server.NewServer()
	ctx := ctxWithCN("rohit")

	start, err := s.StartJob(ctx, &lpaasv1alpha1.StartJobRequest{
		Command: "bash",
		Args:    []string{"-c", "echo one; sleep 1; echo two; sleep 1"},
	})
	require.NoError(t, err)
	require.NotEmpty(t, start.Id)

	require.Eventually(t, func() bool {
		st, err := s.GetStatus(ctx, &lpaasv1alpha1.JobRequest{Id: start.Id})
		return err == nil && st.Status != ""
	}, 2*time.Second, 50*time.Millisecond)

	stream := &fakeStream{ctx: ctx}
	err = s.StreamOutput(&lpaasv1alpha1.StreamRequest{Id: start.Id}, stream)
	require.NoError(t, err)

	output := stream.all()
	require.Contains(t, output, "one")
	require.Contains(t, output, "two")
}
