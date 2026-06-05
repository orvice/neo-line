package connectapi

import (
	"context"

	"connectrpc.com/connect"
	"github.com/orvice/neo-line/internal/store"
	pb "github.com/orvice/neo-line/pkg/proto/neoline/v1"
)

func (s *Service) ListAuditLogs(ctx context.Context, req *connect.Request[pb.ListAuditLogsRequest]) (*connect.Response[pb.ListAuditLogsResponse], error) {
	filter := store.AuditLogFilter{
		Source:       req.Msg.GetSource(),
		Action:       req.Msg.GetAction(),
		ResourceType: req.Msg.GetResourceType(),
		ResourceID:   req.Msg.GetResourceId(),
		ActorEmail:   req.Msg.GetActorEmail(),
		TokenPrefix:  req.Msg.GetTokenPrefix(),
		StartTime:    tsToTimePtr(req.Msg.GetStartTime()),
		EndTime:      tsToTimePtr(req.Msg.GetEndTime()),
	}
	if success := req.Msg.GetSuccess(); success != nil {
		value := success.GetValue()
		filter.Success = &value
	}
	logs, next, err := s.store.ListAuditLogs(ctx, filter, pageLimit(req.Msg.GetPageSize()), req.Msg.GetPageToken())
	if err != nil {
		return nil, toConnectError(err)
	}
	out := &pb.ListAuditLogsResponse{NextPageToken: next}
	for _, log := range logs {
		out.Logs = append(out.Logs, auditLogToProto(log))
	}
	return connect.NewResponse(out), nil
}
