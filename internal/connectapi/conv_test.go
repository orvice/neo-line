package connectapi

import (
	"testing"

	pb "github.com/orvice/neo-line/pkg/proto/neoline/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestServerFromProtoIgnoresComputedFields(t *testing.T) {
	in := &pb.Server{
		Id:                 "srv_1",
		Name:               "web",
		HealthStatus:       "Healthy",
		LastStatusChangeAt: timestamppb.Now(),
		LastCheckAt:        timestamppb.Now(),
		CreatedAt:          timestamppb.Now(),
		UpdatedAt:          timestamppb.Now(),
	}
	out := serverFromProto(in)
	if out.Name != "web" || out.ID != "srv_1" {
		t.Fatalf("client fields not copied: %+v", out)
	}
	if out.HealthStatus != "" {
		t.Fatalf("HealthStatus should not be client-assignable, got %q", out.HealthStatus)
	}
	if !out.LastStatusChangeAt.IsZero() || !out.LastCheckAt.IsZero() {
		t.Fatal("status/check timestamps should not be client-assignable")
	}
	if !out.CreatedAt.IsZero() || !out.UpdatedAt.IsZero() {
		t.Fatal("created/updated timestamps should not be client-assignable")
	}
}

func TestMonitorFromProtoIgnoresComputedFields(t *testing.T) {
	in := &pb.Monitor{
		Id:          "mon_1",
		Name:        "tcp-check",
		Status:      "Healthy",
		LastCheckAt: timestamppb.Now(),
		Certificate: &pb.CertificateInfo{Subject: "forged"},
		CreatedAt:   timestamppb.Now(),
		UpdatedAt:   timestamppb.Now(),
	}
	out := monitorFromProto(in)
	if out.Name != "tcp-check" || out.ID != "mon_1" {
		t.Fatalf("client fields not copied: %+v", out)
	}
	if out.Status != "" {
		t.Fatalf("Status should not be client-assignable, got %q", out.Status)
	}
	if out.Certificate != nil {
		t.Fatal("Certificate should not be client-assignable")
	}
	if !out.LastCheckAt.IsZero() || !out.CreatedAt.IsZero() || !out.UpdatedAt.IsZero() {
		t.Fatal("computed timestamps should not be client-assignable")
	}
}
