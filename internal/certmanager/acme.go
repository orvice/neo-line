package certmanager

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	corelog "butterfly.orx.me/core/log"
	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"
	"github.com/orvice/neo-line/internal/store"
)

// DirectoryMeta holds ACME directory metadata needed before registration.
type DirectoryMeta struct {
	TermsOfService string
}

// IssueRequest carries parameters for ACME certificate issuance.
type IssueRequest struct {
	Issuer               store.CertificateIssuer
	Domains              []string
	KeyType              string
	DNS                  challenge.Provider
	OperationID          string
	ManagedCertificateID string
	AttemptCount         uint32
}

// IssueResult is raw PEM material returned by ACME issuance before validation.
type IssueResult struct {
	FullchainPEM  []byte
	PrivateKeyPEM []byte
}

// ACMEClient fetches directory metadata, registers ACME accounts, issues and revokes certificates.
type ACMEClient interface {
	FetchDirectory(ctx context.Context, directoryURL string) (DirectoryMeta, error)
	RegisterAccount(ctx context.Context, issuer store.CertificateIssuer) error
	IssueCertificate(ctx context.Context, req IssueRequest) (IssueResult, error)
	RevokeCertificate(ctx context.Context, issuer store.CertificateIssuer, leafCertPEM []byte, reason *uint) error
}

// LegoACMEClient registers accounts through go-acme/lego/v4 using the system
// root trust store for HTTPS directory endpoints.
type LegoACMEClient struct {
	httpClient *http.Client
	logger     *slog.Logger
}

func NewLegoACMEClient(httpClient *http.Client) *LegoACMEClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &LegoACMEClient{
		httpClient: httpClient,
		logger:     corelog.FromContext(context.Background()).With("component", "certmanager"),
	}
}

type legoUser struct {
	email        string
	registration *registration.Resource
	key          crypto.PrivateKey
}

func (u *legoUser) GetEmail() string                        { return u.email }
func (u *legoUser) GetRegistration() *registration.Resource { return u.registration }
func (u *legoUser) GetPrivateKey() crypto.PrivateKey        { return u.key }

func parseAccountKeyPEM(pemData string) (crypto.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, errors.New("invalid account key PEM")
	}
	switch block.Type {
	case "EC PRIVATE KEY":
		return x509.ParseECPrivateKey(block.Bytes)
	case "PRIVATE KEY":
		return x509.ParsePKCS8PrivateKey(block.Bytes)
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	default:
		return nil, fmt.Errorf("unsupported account key type %q", block.Type)
	}
}

func (c *LegoACMEClient) newLegoClient(directoryURL, email string, key crypto.PrivateKey) (*lego.Client, error) {
	user := &legoUser{email: email, key: key}
	config := lego.NewConfig(user)
	config.CADirURL = directoryURL
	config.Certificate.KeyType = certcrypto.EC256
	config.HTTPClient = c.httpClient
	return lego.NewClient(config)
}

func (c *LegoACMEClient) FetchDirectory(ctx context.Context, directoryURL string) (DirectoryMeta, error) {
	client, err := c.newLegoClient(directoryURL, "preview@invalid.local", generatePreviewKey())
	if err != nil {
		return DirectoryMeta{}, err
	}
	select {
	case <-ctx.Done():
		return DirectoryMeta{}, ctx.Err()
	default:
	}
	return DirectoryMeta{TermsOfService: client.GetToSURL()}, nil
}

func generatePreviewKey() crypto.PrivateKey {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	return key
}

func (c *LegoACMEClient) RegisterAccount(ctx context.Context, issuer store.CertificateIssuer) error {
	key, err := parseAccountKeyPEM(issuer.AccountKeyPEM)
	if err != nil {
		return err
	}
	client, err := c.newLegoClient(issuer.DirectoryURL, issuer.Email, key)
	if err != nil {
		return err
	}
	client.Challenge.SetDNS01Provider(noopDNSProvider{})

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if issuer.EABKid != "" && issuer.EABHMAC != "" {
		_, err = client.Registration.RegisterWithExternalAccountBinding(registration.RegisterEABOptions{
			TermsOfServiceAgreed: true,
			Kid:                  issuer.EABKid,
			HmacEncoded:          issuer.EABHMAC,
		})
		return err
	}
	_, err = client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
	return err
}

func legoCertKeyType(keyType string) certcrypto.KeyType {
	switch keyType {
	case store.CertKeyTypeRSA2048:
		return certcrypto.RSA2048
	default:
		return certcrypto.EC256
	}
}

func (c *LegoACMEClient) IssueCertificate(ctx context.Context, req IssueRequest) (IssueResult, error) {
	key, err := parseAccountKeyPEM(req.Issuer.AccountKeyPEM)
	if err != nil {
		return IssueResult{}, withOperationStage("parse_account_key", err)
	}
	user := &legoUser{email: req.Issuer.Email, key: key}
	config := lego.NewConfig(user)
	config.CADirURL = req.Issuer.DirectoryURL
	config.Certificate.KeyType = legoCertKeyType(req.KeyType)
	config.HTTPClient = newACMEDiagnosticClient(c.httpClient, c.logger, req)
	client, err := lego.NewClient(config)
	if err != nil {
		return IssueResult{}, withOperationStage("create_acme_client", err)
	}
	if req.DNS == nil {
		return IssueResult{}, withOperationStage("validate_dns_provider", errors.New("dns provider is required"))
	}
	if err := client.Challenge.SetDNS01Provider(req.DNS); err != nil {
		return IssueResult{}, withOperationStage("configure_dns_challenge", err)
	}

	select {
	case <-ctx.Done():
		return IssueResult{}, withOperationStage("acme_context", ctx.Err())
	default:
	}

	certKey, err := generateCertificateKey(req.KeyType)
	if err != nil {
		return IssueResult{}, withOperationStage("generate_certificate_key", err)
	}
	resource, err := client.Certificate.Obtain(certificate.ObtainRequest{
		Domains:    req.Domains,
		Bundle:     true,
		PrivateKey: certKey,
	})
	if err != nil {
		return IssueResult{}, withOperationStage("acme_obtain", err)
	}
	fullchain, err := assembleFullchain(resource.Certificate, resource.IssuerCertificate)
	if err != nil {
		return IssueResult{}, withOperationStage("assemble_fullchain", err)
	}
	keyPEM, err := marshalPrivateKeyPKCS8(certKey)
	if err != nil {
		return IssueResult{}, withOperationStage("marshal_certificate_key", err)
	}
	return IssueResult{FullchainPEM: fullchain, PrivateKeyPEM: keyPEM}, nil
}

func (c *LegoACMEClient) RevokeCertificate(ctx context.Context, issuer store.CertificateIssuer, leafCertPEM []byte, reason *uint) error {
	key, err := parseAccountKeyPEM(issuer.AccountKeyPEM)
	if err != nil {
		return err
	}
	client, err := c.newLegoClient(issuer.DirectoryURL, issuer.Email, key)
	if err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if reason != nil {
		return client.Certificate.RevokeWithReason(leafCertPEM, reason)
	}
	return client.Certificate.Revoke(leafCertPEM)
}

type noopDNSProvider struct{}

func (noopDNSProvider) Present(_, _, _ string) error { return nil }
func (noopDNSProvider) CleanUp(_, _, _ string) error { return nil }

// SetLogger configures structured ACME request logging.
func (c *LegoACMEClient) SetLogger(logger *slog.Logger) {
	if c != nil && logger != nil {
		c.logger = logger
	}
}

// SetHTTPClient replaces the HTTP client (used by tests with httptest servers).
func (c *LegoACMEClient) SetHTTPClient(client *http.Client) {
	if client != nil {
		c.httpClient = client
	}
}

// SetDirectoryBaseURL is a test helper to redirect lego to a fake ACME server.
func SetDirectoryBaseURL(config *lego.Config, baseURL string) {
	config.CADirURL = strings.TrimRight(baseURL, "/") + "/directory"
}

var _ challenge.Provider = noopDNSProvider{}
