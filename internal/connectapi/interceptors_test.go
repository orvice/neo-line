package connectapi

import (
	"testing"

	"github.com/orvice/neo-line/pkg/proto/neoline/v1/neolinev1connect"
)

func TestDataServiceRequiresAdmin(t *testing.T) {
	for _, procedure := range []string{
		neolinev1connect.DataServiceExportConfigProcedure,
		neolinev1connect.DataServiceImportConfigProcedure,
	} {
		if !requiresAdmin(procedure) {
			t.Errorf("%s must require the admin role", procedure)
		}
	}
}

func TestDataServiceAuditMapping(t *testing.T) {
	if got := auditAction(neolinev1connect.DataServiceExportConfigProcedure); got != "export" {
		t.Errorf("export action = %q, want export", got)
	}
	if got := auditAction(neolinev1connect.DataServiceImportConfigProcedure); got != "import" {
		t.Errorf("import action = %q, want import", got)
	}
	for _, procedure := range []string{
		neolinev1connect.DataServiceExportConfigProcedure,
		neolinev1connect.DataServiceImportConfigProcedure,
	} {
		if got := auditResourceType(procedure); got != "config" {
			t.Errorf("%s resource type = %q, want config", procedure, got)
		}
	}
}

func TestExistingAdminAndAuditRulesUnchanged(t *testing.T) {
	if requiresAdmin(neolinev1connect.ServerServiceListServersProcedure) {
		t.Error("read procedures must stay available to non-admin sessions")
	}
	if !requiresAdmin(neolinev1connect.ServerServiceCreateServerProcedure) {
		t.Error("Create* procedures must keep requiring admin")
	}
	if got := auditAction(neolinev1connect.ServerServiceCreateServerProcedure); got != "create" {
		t.Errorf("create action = %q", got)
	}
}
