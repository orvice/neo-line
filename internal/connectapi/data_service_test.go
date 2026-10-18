package connectapi

import (
	"context"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/gin-gonic/gin"
	"github.com/orvice/neo-line/internal/configio"
	"github.com/orvice/neo-line/internal/store"
	pb "github.com/orvice/neo-line/pkg/proto/neoline/v1"
	"github.com/orvice/neo-line/pkg/proto/neoline/v1/neolinev1connect"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// sessionStore embeds the Store interface so only GetSession needs to be
// implemented; any other call would panic, which proves the request was
// rejected before reaching the store.
type sessionStore struct {
	store.Store
	sessions map[string]store.Session
}

func (s sessionStore) GetSession(_ context.Context, token string) (store.Session, error) {
	if sess, ok := s.sessions[token]; ok {
		return sess, nil
	}
	return store.Session{}, mongo.ErrNoDocuments
}

func newDataClient(t *testing.T) neolinev1connect.DataServiceClient {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	Register(r, sessionStore{sessions: map[string]store.Session{
		"admin-token":  {Token: "admin-token", Role: store.RoleAdmin, Email: "a@example.com"},
		"viewer-token": {Token: "viewer-token", Role: "viewer", Email: "v@example.com"},
	}}, nil, nil)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return neolinev1connect.NewDataServiceClient(srv.Client(), srv.URL+BasePath)
}

func withToken[T any](msg *T, token string) *connect.Request[T] {
	req := connect.NewRequest(msg)
	if token != "" {
		req.Header().Set("Authorization", "Bearer "+token)
	}
	return req
}

func TestDataServiceAuth(t *testing.T) {
	client := newDataClient(t)
	ctx := context.Background()

	for _, tc := range []struct {
		name  string
		token string
		want  connect.Code
	}{
		{"no token", "", connect.CodeUnauthenticated},
		{"unknown token", "nope", connect.CodeUnauthenticated},
		{"non-admin", "viewer-token", connect.CodePermissionDenied},
	} {
		t.Run("export "+tc.name, func(t *testing.T) {
			_, err := client.ExportConfig(ctx, withToken(&pb.ExportConfigRequest{}, tc.token))
			if got := connect.CodeOf(err); got != tc.want {
				t.Fatalf("code = %v, want %v (err %v)", got, tc.want, err)
			}
		})
		t.Run("import "+tc.name, func(t *testing.T) {
			_, err := client.ImportConfig(ctx, withToken(&pb.ImportConfigRequest{Data: []byte(`{"version":1}`), Mode: pb.ImportMode_IMPORT_MODE_MERGE}, tc.token))
			if got := connect.CodeOf(err); got != tc.want {
				t.Fatalf("code = %v, want %v (err %v)", got, tc.want, err)
			}
		})
	}
}

func TestImportConfigRejectsInvalidRequests(t *testing.T) {
	client := newDataClient(t)
	ctx := context.Background()

	for _, tc := range []struct {
		name string
		req  *pb.ImportConfigRequest
	}{
		{"mode unspecified", &pb.ImportConfigRequest{Data: []byte(`{"version":1}`)}},
		{"malformed json", &pb.ImportConfigRequest{Data: []byte(`{`), Mode: pb.ImportMode_IMPORT_MODE_MERGE}},
		{"future version", &pb.ImportConfigRequest{Data: []byte(`{"version":2}`), Mode: pb.ImportMode_IMPORT_MODE_UPSERT}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := client.ImportConfig(ctx, withToken(tc.req, "admin-token"))
			if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
				t.Fatalf("code = %v, want invalid_argument (err %v)", got, err)
			}
		})
	}
}

func TestImportModeFromProto(t *testing.T) {
	for in, want := range map[pb.ImportMode]configio.Mode{
		pb.ImportMode_IMPORT_MODE_UNSPECIFIED: 0,
		pb.ImportMode_IMPORT_MODE_MERGE:       configio.ModeMerge,
		pb.ImportMode_IMPORT_MODE_UPSERT:      configio.ModeUpsert,
	} {
		if got := importModeFromProto(in); got != want {
			t.Errorf("importModeFromProto(%v) = %v, want %v", in, got, want)
		}
	}
}

func TestReportToProto(t *testing.T) {
	got := reportToProto(configio.Report{
		DryRun: true,
		Summary: configio.Summary{
			Servers:  configio.Counts{Created: 1, Updated: 2, Skipped: 3, Failed: 4},
			Monitors: configio.Counts{Created: 5},
		},
		Errors: []configio.Issue{{ResourceType: configio.ResourceServer, ID: "srv_1", Name: "web", Message: "boom"}},
	})
	if !got.GetDryRun() {
		t.Error("dry_run not propagated")
	}
	s := got.GetSummary().GetServers()
	if s.GetCreated() != 1 || s.GetUpdated() != 2 || s.GetSkipped() != 3 || s.GetFailed() != 4 {
		t.Errorf("servers counts = %v", s)
	}
	if got.GetSummary().GetMonitors().GetCreated() != 5 || got.GetSummary().GetSettings() == nil {
		t.Errorf("summary = %v", got.GetSummary())
	}
	if len(got.GetErrors()) != 1 || got.GetErrors()[0].GetId() != "srv_1" || got.GetErrors()[0].GetMessage() != "boom" {
		t.Errorf("errors = %v", got.GetErrors())
	}
}
