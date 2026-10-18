package certmanager

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-acme/lego/v4/challenge"
	"github.com/orvice/neo-line/internal/store"
)

func diagnosticTestLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, nil))
}

func TestCertificateFailureLogContainsStageAndNoSecret(t *testing.T) {
	st := newManagedCertFakeStore()
	dns := NewFakeDNSProvider(&fakeDNSZone{name: "example.com"})
	_, opID := seedIssueTestStore(st, []string{"example.com"}, store.CertKeyTypeECP256, false)
	m := newIssueTestManager(t, st, failIssueACME("cloudflare token super-secret-value"), dns)

	var logs bytes.Buffer
	m.SetLogger(diagnosticTestLogger(&logs))
	m.runOperation(context.Background(), opID)

	output := logs.String()
	for _, want := range []string{
		`"operation_id":"cop_1"`,
		`"managed_certificate_id":"mcert_1"`,
		`"error_stage":"acme_obtain"`,
		`"final_status":"Pending"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("logs missing %s: %s", want, output)
		}
	}
	if strings.Contains(output, "super-secret-value") {
		t.Fatalf("logs leaked secret: %s", output)
	}
}

func TestDNSPresentFailureLogUsesDNSStage(t *testing.T) {
	st := newManagedCertFakeStore()
	dns := NewFakeDNSProvider()
	_, opID := seedIssueTestStore(st, []string{"example.com"}, store.CertKeyTypeECP256, false)
	acme := &fakeIssueACME{issueFn: func(_ context.Context, req IssueRequest) (IssueResult, error) {
		return IssueResult{}, req.DNS.Present("example.com", "challenge-token", "key-auth-secret")
	}}
	m := newIssueTestManager(t, st, acme, dns)

	var logs bytes.Buffer
	m.SetLogger(diagnosticTestLogger(&logs))
	m.runOperation(context.Background(), opID)

	output := logs.String()
	if !strings.Contains(output, `"error_stage":"dns_present"`) {
		t.Fatalf("logs missing dns_present stage: %s", output)
	}
	if !strings.Contains(output, `"error_class":"dns_error"`) {
		t.Fatalf("logs missing dns error class: %s", output)
	}
	if strings.Contains(output, "challenge-token") || strings.Contains(output, "key-auth-secret") {
		t.Fatalf("logs leaked challenge material: %s", output)
	}
}

func TestCloudflareVerificationLogContainsStatusAndCodeWithoutToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"success":false,"errors":[{"code":10000,"message":"Invalid API Token"}]}`))
	}))
	defer server.Close()

	client := NewCloudflareClient(server.Client())
	client.baseURL = server.URL
	var logs bytes.Buffer
	client.SetLogger(diagnosticTestLogger(&logs))
	if err := client.VerifyCloudflareToken(context.Background(), "cloudflare-secret-token"); err == nil {
		t.Fatal("expected token verification failure")
	}

	output := logs.String()
	if !strings.Contains(output, `"http_status":403`) {
		t.Fatalf("logs missing HTTP status: %s", output)
	}
	if !strings.Contains(output, `"cloudflare_error_codes":[10000]`) {
		t.Fatalf("logs missing Cloudflare error code: %s", output)
	}
	if strings.Contains(output, "cloudflare-secret-token") || strings.Contains(output, "Invalid API Token") {
		t.Fatalf("logs leaked token or response message: %s", output)
	}
}

func TestACMEDiagnosticLogOmitsFullURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	var logs bytes.Buffer
	client := newACMEDiagnosticClient(server.Client(), diagnosticTestLogger(&logs), IssueRequest{
		OperationID:          "cop_1",
		ManagedCertificateID: "mcert_1",
		AttemptCount:         2,
	})
	requestURL := server.URL + "/directory/order-secret-value"
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, requestURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Do(request); err != nil {
		t.Fatal(err)
	}

	output := logs.String()
	if !strings.Contains(output, `"acme_request_kind":"other"`) {
		t.Fatalf("logs missing request kind: %s", output)
	}
	if strings.Contains(output, "order-secret-value") || strings.Contains(output, requestURL) {
		t.Fatalf("logs leaked ACME URL: %s", output)
	}
}

func TestOperationStagePreservesWrappedStage(t *testing.T) {
	base := errors.New("provider failed")
	inner := withOperationStage("dns_present", base)
	outer := withOperationStage("acme_obtain", inner)
	if got := operationStageOf(outer); got != "dns_present" {
		t.Fatalf("stage = %q, want dns_present", got)
	}
	if !errors.Is(outer, base) {
		t.Fatal("staged error should unwrap to the original error")
	}
}

var _ challenge.Provider = (*FakeDNSProvider)(nil)
