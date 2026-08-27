package connectapi

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"
	"github.com/orvice/neo-line/internal/certmanager"
	pb "github.com/orvice/neo-line/pkg/proto/neoline/v1"
)

func (s *Service) ListManagedCertificates(ctx context.Context, req *connect.Request[pb.ListManagedCertificatesRequest]) (*connect.Response[pb.ListManagedCertificatesResponse], error) {
	certs, next, err := s.certManager.ListManagedCertificates(ctx, pageLimit(req.Msg.GetPageSize()), req.Msg.GetPageToken())
	if err != nil {
		return nil, toConnectError(err)
	}
	out := &pb.ListManagedCertificatesResponse{NextPageToken: next}
	for _, c := range certs {
		out.Certificates = append(out.Certificates, managedCertificateToProto(c))
	}
	return connect.NewResponse(out), nil
}

func (s *Service) CreateManagedCertificate(ctx context.Context, req *connect.Request[pb.CreateManagedCertificateRequest]) (*connect.Response[pb.CreateManagedCertificateResponse], error) {
	input, err := managedCertificateInputFromProto(req.Msg.GetCertificate())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	created, err := s.certManager.CreateManagedCertificate(ctx, input)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.CreateManagedCertificateResponse{
		Certificate: managedCertificateToProto(created),
	}), nil
}

func (s *Service) GetManagedCertificate(ctx context.Context, req *connect.Request[pb.GetManagedCertificateRequest]) (*connect.Response[pb.GetManagedCertificateResponse], error) {
	cert, err := s.certManager.GetManagedCertificate(ctx, req.Msg.GetManagedCertificateId())
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.GetManagedCertificateResponse{
		Certificate: managedCertificateToProto(cert),
	}), nil
}

func (s *Service) UpdateManagedCertificate(ctx context.Context, req *connect.Request[pb.UpdateManagedCertificateRequest]) (*connect.Response[pb.UpdateManagedCertificateResponse], error) {
	input, err := managedCertificateInputFromProto(req.Msg.GetCertificate())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	updated, err := s.certManager.UpdateManagedCertificate(ctx, req.Msg.GetManagedCertificateId(), input)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.UpdateManagedCertificateResponse{
		Certificate: managedCertificateToProto(updated),
	}), nil
}

func (s *Service) SubmitIssueOperation(ctx context.Context, req *connect.Request[pb.SubmitIssueOperationRequest]) (*connect.Response[pb.SubmitIssueOperationResponse], error) {
	op, err := s.certManager.SubmitIssueOperation(ctx, req.Msg.GetManagedCertificateId())
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.SubmitIssueOperationResponse{
		Operation: certificateOperationToProto(op),
	}), nil
}

func (s *Service) SubmitRenewOperation(ctx context.Context, req *connect.Request[pb.SubmitRenewOperationRequest]) (*connect.Response[pb.SubmitRenewOperationResponse], error) {
	op, err := s.certManager.SubmitRenewOperation(ctx, req.Msg.GetManagedCertificateId())
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.SubmitRenewOperationResponse{
		Operation: certificateOperationToProto(op),
	}), nil
}

func (s *Service) GetCertificateBundle(ctx context.Context, req *connect.Request[pb.GetCertificateBundleRequest]) (*connect.Response[pb.GetCertificateBundleResponse], error) {
	slot := versionSlotFromProto(req.Msg.GetVersionSlot())
	bundle, err := s.certManager.GetCertificateBundle(ctx, req.Msg.GetManagedCertificateId(), slot)
	if err != nil {
		return nil, toConnectError(err)
	}
	resp := connect.NewResponse(&pb.GetCertificateBundleResponse{
		Bundle: certificateBundleToProto(bundle),
	})
	resp.Header().Set("Cache-Control", "no-store")
	resp.Header().Set("X-Certificate-Version-Id", bundle.VersionID)
	return resp, nil
}

func (s *Service) ActivatePreviousVersion(ctx context.Context, req *connect.Request[pb.ActivatePreviousVersionRequest]) (*connect.Response[pb.ActivatePreviousVersionResponse], error) {
	cert, err := s.certManager.ActivatePreviousVersion(ctx, req.Msg.GetManagedCertificateId(), req.Msg.GetVersionId())
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.ActivatePreviousVersionResponse{
		Certificate: managedCertificateToProto(cert),
	}), nil
}

func versionSlotFromProto(slot pb.CertificateVersionSlot) string {
	switch slot {
	case pb.CertificateVersionSlot_CERTIFICATE_VERSION_SLOT_PREVIOUS:
		return certmanager.VersionSlotPrevious
	default:
		return certmanager.VersionSlotActive
	}
}

func managedCertificateInputFromProto(p *pb.ManagedCertificate) (certmanager.ManagedCertificateInput, error) {
	if p == nil {
		return certmanager.ManagedCertificateInput{}, errors.New("certificate is required")
	}
	if strings.TrimSpace(p.GetName()) == "" {
		return certmanager.ManagedCertificateInput{}, certmanager.ErrManagedCertificateNameRequired
	}
	input := certmanager.ManagedCertificateInput{
		Name:                 strings.TrimSpace(p.GetName()),
		Domains:              append([]string(nil), p.GetDomains()...),
		CertificateIssuerID:  strings.TrimSpace(p.GetCertificateIssuerId()),
		DNSProviderAccountID: strings.TrimSpace(p.GetDnsProviderAccountId()),
		KeyType:              keyTypeFromProto(p.GetKeyType()),
		RenewBeforeDays:      p.GetRenewBeforeDays(),
		NotifyGroupIDs:       append([]string(nil), p.GetNotifyGroupIds()...),
		ServerIDs:            append([]string(nil), p.GetServerIds()...),
	}
	if p.AutoRenewEnabled != nil {
		v := p.GetAutoRenewEnabled()
		input.AutoRenewEnabled = &v
	}
	return input, nil
}

func keyTypeFromProto(t pb.CertificateKeyType) string {
	switch t {
	case pb.CertificateKeyType_CERTIFICATE_KEY_TYPE_RSA_2048:
		return "rsa_2048"
	case pb.CertificateKeyType_CERTIFICATE_KEY_TYPE_EC_P256:
		return "ec_p256"
	default:
		return ""
	}
}
