package connectapi

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"
	"github.com/orvice/neo-line/internal/certmanager"
	pb "github.com/orvice/neo-line/pkg/proto/neoline/v1"
)

func (s *Service) ListCertificateIssuers(ctx context.Context, req *connect.Request[pb.ListCertificateIssuersRequest]) (*connect.Response[pb.ListCertificateIssuersResponse], error) {
	issuers, next, err := s.certManager.ListCertificateIssuers(ctx, pageLimit(req.Msg.GetPageSize()), req.Msg.GetPageToken())
	if err != nil {
		return nil, toConnectError(err)
	}
	out := &pb.ListCertificateIssuersResponse{NextPageToken: next}
	for _, i := range issuers {
		out.Issuers = append(out.Issuers, certificateIssuerToProto(i))
	}
	return connect.NewResponse(out), nil
}

func (s *Service) GetCertificateIssuerDirectoryPreview(ctx context.Context, req *connect.Request[pb.GetCertificateIssuerDirectoryPreviewRequest]) (*connect.Response[pb.GetCertificateIssuerDirectoryPreviewResponse], error) {
	preview, err := s.certManager.GetCertificateIssuerDirectoryPreview(ctx, req.Msg.GetCaType(), req.Msg.GetCustomDirectoryUrl())
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.GetCertificateIssuerDirectoryPreviewResponse{
		Preview: certificateIssuerDirectoryPreviewToProto(preview),
	}), nil
}

func (s *Service) CreateCertificateIssuer(ctx context.Context, req *connect.Request[pb.CreateCertificateIssuerRequest]) (*connect.Response[pb.CreateCertificateIssuerResponse], error) {
	input, err := issuerInputFromCreate(req.Msg)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	created, err := s.certManager.CreateCertificateIssuer(ctx, input)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.CreateCertificateIssuerResponse{Issuer: certificateIssuerToProto(created)}), nil
}

func (s *Service) GetCertificateIssuer(ctx context.Context, req *connect.Request[pb.GetCertificateIssuerRequest]) (*connect.Response[pb.GetCertificateIssuerResponse], error) {
	issuer, err := s.certManager.GetCertificateIssuer(ctx, req.Msg.GetCertificateIssuerId())
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.GetCertificateIssuerResponse{Issuer: certificateIssuerToProto(issuer)}), nil
}

func (s *Service) UpdateCertificateIssuer(ctx context.Context, req *connect.Request[pb.UpdateCertificateIssuerRequest]) (*connect.Response[pb.UpdateCertificateIssuerResponse], error) {
	input, err := issuerInputFromUpdate(req.Msg)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	updated, err := s.certManager.UpdateCertificateIssuer(ctx, req.Msg.GetCertificateIssuerId(), input)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.UpdateCertificateIssuerResponse{Issuer: certificateIssuerToProto(updated)}), nil
}

func (s *Service) DeleteCertificateIssuer(ctx context.Context, req *connect.Request[pb.DeleteCertificateIssuerRequest]) (*connect.Response[pb.DeleteCertificateIssuerResponse], error) {
	if err := s.certManager.DeleteCertificateIssuer(ctx, req.Msg.GetCertificateIssuerId()); err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.DeleteCertificateIssuerResponse{}), nil
}

func (s *Service) RetryCertificateIssuerRegistration(ctx context.Context, req *connect.Request[pb.RetryCertificateIssuerRegistrationRequest]) (*connect.Response[pb.RetryCertificateIssuerRegistrationResponse], error) {
	issuer, err := s.certManager.RetryCertificateIssuerRegistration(ctx, req.Msg.GetCertificateIssuerId())
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.RetryCertificateIssuerRegistrationResponse{Issuer: certificateIssuerToProto(issuer)}), nil
}

func issuerInputFromCreate(msg *pb.CreateCertificateIssuerRequest) (certmanager.IssuerInput, error) {
	if msg == nil || msg.GetIssuer() == nil {
		return certmanager.IssuerInput{}, errors.New("issuer is required")
	}
	if strings.TrimSpace(msg.GetIssuer().GetName()) == "" {
		return certmanager.IssuerInput{}, errors.New("name is required")
	}
	if strings.TrimSpace(msg.GetIssuer().GetEmail()) == "" {
		return certmanager.IssuerInput{}, errors.New("email is required")
	}
	if strings.TrimSpace(msg.GetIssuer().GetCaType()) == "" {
		return certmanager.IssuerInput{}, errors.New("ca_type is required")
	}
	return certmanager.IssuerInput{
		Name:                 strings.TrimSpace(msg.GetIssuer().GetName()),
		CAType:               msg.GetIssuer().GetCaType(),
		CustomDirectoryURL:   msg.GetCustomDirectoryUrl(),
		Email:                strings.TrimSpace(msg.GetIssuer().GetEmail()),
		AccountKeyPEM:        msg.GetAccountKeyPem(),
		EABKid:               msg.GetEabKid(),
		EABHMAC:              msg.GetEabHmac(),
		TermsOfServiceAgreed: msg.GetTermsOfServiceAgreed(),
	}, nil
}

func issuerInputFromUpdate(msg *pb.UpdateCertificateIssuerRequest) (certmanager.IssuerInput, error) {
	if msg == nil || msg.GetIssuer() == nil {
		return certmanager.IssuerInput{}, errors.New("issuer is required")
	}
	if strings.TrimSpace(msg.GetIssuer().GetName()) == "" {
		return certmanager.IssuerInput{}, errors.New("name is required")
	}
	return certmanager.IssuerInput{
		Name:               strings.TrimSpace(msg.GetIssuer().GetName()),
		CAType:             msg.GetIssuer().GetCaType(),
		CustomDirectoryURL: msg.GetCustomDirectoryUrl(),
		Email:              strings.TrimSpace(msg.GetIssuer().GetEmail()),
		AccountKeyPEM:      msg.GetAccountKeyPem(),
		EABKid:             msg.GetEabKid(),
		EABHMAC:            msg.GetEabHmac(),
	}, nil
}
