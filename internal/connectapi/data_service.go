package connectapi

import (
	"context"
	"errors"
	"time"

	"connectrpc.com/connect"
	"github.com/orvice/neo-line/internal/configio"
	pb "github.com/orvice/neo-line/pkg/proto/neoline/v1"
)

// ExportConfig returns all monitoring configuration as a JSON document. The
// document includes notify channel secrets in plaintext; the procedure is
// admin only.
func (s *Service) ExportConfig(ctx context.Context, _ *connect.Request[pb.ExportConfigRequest]) (*connect.Response[pb.ExportConfigResponse], error) {
	data, err := configio.Export(ctx, s.store, time.Now().UTC())
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.ExportConfigResponse{Data: data}), nil
}

// ImportConfig applies a JSON document produced by ExportConfig. Item-level
// failures are reported in the response; only invalid requests return an RPC
// error.
func (s *Service) ImportConfig(ctx context.Context, req *connect.Request[pb.ImportConfigRequest]) (*connect.Response[pb.ImportConfigResponse], error) {
	report, err := configio.Import(ctx, s.store, req.Msg.GetData(), configio.Options{
		Mode:   importModeFromProto(req.Msg.GetMode()),
		DryRun: req.Msg.GetDryRun(),
	})
	if err != nil {
		if errors.Is(err, configio.ErrInvalidInput) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		return nil, toConnectError(err)
	}
	return connect.NewResponse(reportToProto(report)), nil
}

func importModeFromProto(m pb.ImportMode) configio.Mode {
	switch m {
	case pb.ImportMode_IMPORT_MODE_MERGE:
		return configio.ModeMerge
	case pb.ImportMode_IMPORT_MODE_UPSERT:
		return configio.ModeUpsert
	default:
		return 0
	}
}

func countsToProto(c configio.Counts) *pb.ImportCounts {
	return &pb.ImportCounts{Created: c.Created, Updated: c.Updated, Skipped: c.Skipped, Failed: c.Failed}
}

func reportToProto(r configio.Report) *pb.ImportConfigResponse {
	out := &pb.ImportConfigResponse{
		DryRun: r.DryRun,
		Summary: &pb.ImportSummary{
			Settings:      countsToProto(r.Summary.Settings),
			NotifyGroups:  countsToProto(r.Summary.NotifyGroups),
			MonitorGroups: countsToProto(r.Summary.MonitorGroups),
			Servers:       countsToProto(r.Summary.Servers),
			Monitors:      countsToProto(r.Summary.Monitors),
		},
	}
	for _, e := range r.Errors {
		out.Errors = append(out.Errors, &pb.ImportError{ResourceType: e.ResourceType, Id: e.ID, Name: e.Name, Message: e.Message})
	}
	return out
}
