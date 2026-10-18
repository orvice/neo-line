package connectapi

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"
	"github.com/orvice/neo-line/internal/certmanager"
	pb "github.com/orvice/neo-line/pkg/proto/neoline/v1"
)

func (s *Service) ListDNSProviderAccounts(ctx context.Context, req *connect.Request[pb.ListDNSProviderAccountsRequest]) (*connect.Response[pb.ListDNSProviderAccountsResponse], error) {
	accounts, next, err := s.certManager.ListDNSProviderAccounts(ctx, pageLimit(req.Msg.GetPageSize()), req.Msg.GetPageToken())
	if err != nil {
		return nil, toConnectError(err)
	}
	out := &pb.ListDNSProviderAccountsResponse{NextPageToken: next}
	for _, a := range accounts {
		out.Accounts = append(out.Accounts, dnsProviderAccountToProto(a))
	}
	return connect.NewResponse(out), nil
}

func (s *Service) CreateDNSProviderAccount(ctx context.Context, req *connect.Request[pb.CreateDNSProviderAccountRequest]) (*connect.Response[pb.CreateDNSProviderAccountResponse], error) {
	input, err := dnsAccountInputFromCreate(req.Msg)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	created, err := s.certManager.CreateDNSProviderAccount(ctx, input)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.CreateDNSProviderAccountResponse{Account: dnsProviderAccountToProto(created)}), nil
}

func (s *Service) GetDNSProviderAccount(ctx context.Context, req *connect.Request[pb.GetDNSProviderAccountRequest]) (*connect.Response[pb.GetDNSProviderAccountResponse], error) {
	account, err := s.certManager.GetDNSProviderAccount(ctx, req.Msg.GetDnsProviderAccountId())
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.GetDNSProviderAccountResponse{Account: dnsProviderAccountToProto(account)}), nil
}

func (s *Service) UpdateDNSProviderAccount(ctx context.Context, req *connect.Request[pb.UpdateDNSProviderAccountRequest]) (*connect.Response[pb.UpdateDNSProviderAccountResponse], error) {
	input, err := dnsAccountInputFromUpdate(req.Msg)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	updated, err := s.certManager.UpdateDNSProviderAccount(ctx, req.Msg.GetDnsProviderAccountId(), input)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.UpdateDNSProviderAccountResponse{Account: dnsProviderAccountToProto(updated)}), nil
}

func (s *Service) DeleteDNSProviderAccount(ctx context.Context, req *connect.Request[pb.DeleteDNSProviderAccountRequest]) (*connect.Response[pb.DeleteDNSProviderAccountResponse], error) {
	if err := s.certManager.DeleteDNSProviderAccount(ctx, req.Msg.GetDnsProviderAccountId()); err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.DeleteDNSProviderAccountResponse{}), nil
}

func dnsAccountInputFromCreate(msg *pb.CreateDNSProviderAccountRequest) (certmanager.DNSAccountInput, error) {
	if msg == nil || msg.GetAccount() == nil {
		return certmanager.DNSAccountInput{}, errors.New("account is required")
	}
	if strings.TrimSpace(msg.GetAccount().GetName()) == "" {
		return certmanager.DNSAccountInput{}, errors.New("name is required")
	}
	if strings.TrimSpace(msg.GetApiToken()) == "" {
		return certmanager.DNSAccountInput{}, errors.New("api_token is required")
	}
	return certmanager.DNSAccountInput{
		Name:                      strings.TrimSpace(msg.GetAccount().GetName()),
		Provider:                  msg.GetAccount().GetProvider(),
		PropagationTimeoutSeconds: msg.GetAccount().GetPropagationTimeoutSeconds(),
		APIToken:                  msg.GetApiToken(),
	}, nil
}

func dnsAccountInputFromUpdate(msg *pb.UpdateDNSProviderAccountRequest) (certmanager.DNSAccountInput, error) {
	if msg == nil || msg.GetAccount() == nil {
		return certmanager.DNSAccountInput{}, errors.New("account is required")
	}
	if strings.TrimSpace(msg.GetAccount().GetName()) == "" {
		return certmanager.DNSAccountInput{}, errors.New("name is required")
	}
	return certmanager.DNSAccountInput{
		Name:                      strings.TrimSpace(msg.GetAccount().GetName()),
		Provider:                  msg.GetAccount().GetProvider(),
		PropagationTimeoutSeconds: msg.GetAccount().GetPropagationTimeoutSeconds(),
		APIToken:                  msg.GetApiToken(),
	}, nil
}
