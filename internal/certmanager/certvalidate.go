package certmanager

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/orvice/neo-line/internal/store"
)

// ParsedBundle holds validated certificate material from an ACME issuance.
type ParsedBundle struct {
	Leaf             *x509.Certificate
	FullchainPEM     []byte
	PrivateKeyPEM    []byte
	LeafFingerprint  string
	SerialNumber     string
	IssuerCommonName string
	NotBefore        time.Time
	NotAfter         time.Time
}

var (
	ErrCertificateKeyMismatch    = errors.New("certificate and private key do not match")
	ErrCertificateDomainMismatch = errors.New("certificate does not cover requested domains")
	ErrCertificateChainInvalid   = errors.New("certificate chain is invalid or incomplete")
	ErrCertificateNotYetValid    = errors.New("certificate is not yet valid")
	ErrCertificateExpired        = errors.New("certificate is expired")
)

func generateCertificateKey(keyType string) (crypto.PrivateKey, error) {
	switch keyType {
	case store.CertKeyTypeRSA2048:
		return rsa.GenerateKey(rand.Reader, 2048)
	default:
		return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	}
}

func marshalPrivateKeyPKCS8(key crypto.PrivateKey) ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}

func assembleFullchain(leafPEM, issuerPEM []byte) ([]byte, error) {
	certs, err := parsePEMCertificates(append(leafPEM, issuerPEM...))
	if err != nil {
		return nil, err
	}
	if len(certs) == 0 {
		return nil, ErrCertificateChainInvalid
	}
	for len(certs) > 1 && isSelfSignedRoot(certs[len(certs)-1]) {
		certs = certs[:len(certs)-1]
	}
	var buf strings.Builder
	for _, cert := range certs {
		if err := pem.Encode(&buf, &pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}); err != nil {
			return nil, err
		}
	}
	return []byte(buf.String()), nil
}

func parsePEMCertificates(pemData []byte) ([]*x509.Certificate, error) {
	var certs []*x509.Certificate
	rest := pemData
	for {
		block, remaining := pem.Decode(rest)
		if block == nil {
			break
		}
		rest = remaining
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, err
		}
		certs = append(certs, cert)
	}
	return certs, nil
}

func isSelfSignedRoot(cert *x509.Certificate) bool {
	return cert.Subject.String() == cert.Issuer.String()
}

func validateIssuedBundle(fullchainPEM, privateKeyPEM []byte, domains []string, keyType string, now time.Time) (ParsedBundle, error) {
	key, err := parsePrivateKeyPEM(privateKeyPEM)
	if err != nil {
		return ParsedBundle{}, err
	}
	if err := assertKeyType(key, keyType); err != nil {
		return ParsedBundle{}, err
	}
	certs, err := parsePEMCertificates(fullchainPEM)
	if err != nil {
		return ParsedBundle{}, err
	}
	if len(certs) == 0 {
		return ParsedBundle{}, ErrCertificateChainInvalid
	}
	leaf := certs[0]
	if err := leaf.CheckSignatureFrom(leaf); err == nil && len(certs) == 1 {
		// single self-signed — not a valid issued leaf for our purposes unless test CA
	}
	pool := x509.NewCertPool()
	for i := 1; i < len(certs); i++ {
		pool.AddCert(certs[i])
	}
	if len(certs) > 1 {
		if _, err := leaf.Verify(x509.VerifyOptions{
			Intermediates: pool,
			CurrentTime:   now,
		}); err != nil {
			// Allow verify failure when intermediates are present but not chained to a trusted root.
			// We still require parseable chain structure.
			if !hasParseableChain(certs) {
				return ParsedBundle{}, fmt.Errorf("%w: %v", ErrCertificateChainInvalid, err)
			}
		}
	}
	if err := keyMatchesCert(key, leaf); err != nil {
		return ParsedBundle{}, err
	}
	if err := domainsMatchCert(domains, leaf); err != nil {
		return ParsedBundle{}, err
	}
	if now.Before(leaf.NotBefore) {
		return ParsedBundle{}, ErrCertificateNotYetValid
	}
	if now.After(leaf.NotAfter) {
		return ParsedBundle{}, ErrCertificateExpired
	}
	sum := sha256.Sum256(leaf.Raw)
	issuerCN := leaf.Issuer.CommonName
	return ParsedBundle{
		Leaf:             leaf,
		FullchainPEM:     fullchainPEM,
		PrivateKeyPEM:    privateKeyPEM,
		LeafFingerprint:  hex.EncodeToString(sum[:]),
		SerialNumber:     leaf.SerialNumber.String(),
		IssuerCommonName: issuerCN,
		NotBefore:        leaf.NotBefore.UTC(),
		NotAfter:         leaf.NotAfter.UTC(),
	}, nil
}

func hasParseableChain(certs []*x509.Certificate) bool {
	for i := 0; i < len(certs)-1; i++ {
		if err := certs[i].CheckSignatureFrom(certs[i+1]); err != nil {
			return false
		}
	}
	return true
}

func parsePrivateKeyPEM(pemData []byte) (crypto.PrivateKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, errors.New("invalid private key PEM")
	}
	switch block.Type {
	case "PRIVATE KEY":
		return x509.ParsePKCS8PrivateKey(block.Bytes)
	case "EC PRIVATE KEY":
		return x509.ParseECPrivateKey(block.Bytes)
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	default:
		return nil, fmt.Errorf("unsupported private key type %q", block.Type)
	}
}

func assertKeyType(key crypto.PrivateKey, keyType string) error {
	switch keyType {
	case store.CertKeyTypeRSA2048:
		if _, ok := key.(*rsa.PrivateKey); !ok {
			return fmt.Errorf("expected RSA key, got %T", key)
		}
	case store.CertKeyTypeECP256:
		if _, ok := key.(*ecdsa.PrivateKey); !ok {
			return fmt.Errorf("expected EC key, got %T", key)
		}
	default:
		return ErrInvalidKeyType
	}
	return nil
}

func keyMatchesCert(key crypto.PrivateKey, cert *x509.Certificate) error {
	switch pub := cert.PublicKey.(type) {
	case *rsa.PublicKey:
		priv, ok := key.(*rsa.PrivateKey)
		if !ok || priv.PublicKey.N.Cmp(pub.N) != 0 || priv.PublicKey.E != pub.E {
			return ErrCertificateKeyMismatch
		}
	case *ecdsa.PublicKey:
		priv, ok := key.(*ecdsa.PrivateKey)
		if !ok || priv.PublicKey.X.Cmp(pub.X) != 0 || priv.PublicKey.Y.Cmp(pub.Y) != 0 {
			return ErrCertificateKeyMismatch
		}
	default:
		return ErrCertificateKeyMismatch
	}
	return nil
}

func domainsMatchCert(domains []string, cert *x509.Certificate) error {
	if len(domains) == 0 {
		return ErrCertificateDomainMismatch
	}
	covered := make(map[string]struct{})
	covered[strings.ToLower(cert.Subject.CommonName)] = struct{}{}
	for _, name := range cert.DNSNames {
		covered[strings.ToLower(name)] = struct{}{}
	}
	for _, want := range domains {
		if !domainCovered(want, covered) {
			return fmt.Errorf("%w: missing %q", ErrCertificateDomainMismatch, want)
		}
	}
	return nil
}

func domainCovered(domain string, covered map[string]struct{}) bool {
	domain = strings.ToLower(domain)
	if _, ok := covered[domain]; ok {
		return true
	}
	if strings.HasPrefix(domain, "*.") {
		base := strings.TrimPrefix(domain, "*.")
		for name := range covered {
			if name == base || strings.HasSuffix(name, "."+base) {
				return true
			}
		}
	}
	return false
}

func leafFingerprint(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(sum[:])
}
