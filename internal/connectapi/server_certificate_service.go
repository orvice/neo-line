package connectapi

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	pb "github.com/orvice/neo-line/pkg/proto/neoline/v1"
)

// ServerCertService implements ServerCertificateService with nlct_ Bearer auth.
type ServerCertService struct {
	parent *Service
}

func (s *ServerCertService) ListCertificates(ctx context.Context, req *connect.Request[pb.ListCertificatesRequest]) (*connect.Response[pb.ListCertificatesResponse], error) {
	token, ok := certAccessTokenFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("missing bearer token"))
	}
	certs, err := s.parent.certManager.ListServerCertificates(ctx, token.ServerID)
	if err != nil {
		return nil, toConnectError(err)
	}
	out := &pb.ListCertificatesResponse{}
	for _, c := range certs {
		out.Certificates = append(out.Certificates, serverCertificateToProto(c))
	}
	return connect.NewResponse(out), nil
}

func (s *ServerCertService) GetCertificateBundle(ctx context.Context, req *connect.Request[pb.ServerCertificateServiceGetCertificateBundleRequest]) (*connect.Response[pb.ServerCertificateServiceGetCertificateBundleResponse], error) {
	token, ok := certAccessTokenFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("missing bearer token"))
	}
	bundle, err := s.parent.certManager.GetServerCertificateBundle(ctx, token.ServerID, req.Msg.GetManagedCertificateId())
	if err != nil {
		return nil, toConnectError(err)
	}
	resp := connect.NewResponse(&pb.ServerCertificateServiceGetCertificateBundleResponse{
		Bundle: certificateBundleToProto(bundle),
	})
	resp.Header().Set("Cache-Control", "no-store")
	return resp, nil
}
