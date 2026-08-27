package certmanager

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/orvice/neo-line/internal/store"
	"golang.org/x/net/idna"
)

const maxCertificateDomains = 100

// NormalizeDomains trims, lowercases, converts to IDNA ASCII, removes trailing
// dots, validates DNS/wildcard syntax, deduplicates, and preserves order.
// The first domain is the primary domain.
func NormalizeDomains(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("%w: at least one domain is required", ErrInvalidDomains)
	}
	if len(raw) > maxCertificateDomains {
		return nil, ErrTooManyDomains
	}
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		d, err := normalizeDomain(item)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[d]; ok {
			continue
		}
		seen[d] = struct{}{}
		out = append(out, d)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: at least one domain is required", ErrInvalidDomains)
	}
	if len(out) > maxCertificateDomains {
		return nil, ErrTooManyDomains
	}
	return out, nil
}

func normalizeDomain(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", fmt.Errorf("%w: empty domain", ErrInvalidDomains)
	}
	s = strings.TrimSuffix(s, ".")
	s = strings.ToLower(s)
	if s == "" {
		return "", fmt.Errorf("%w: empty domain", ErrInvalidDomains)
	}

	var ascii string
	if strings.HasPrefix(s, "*.") {
		suffix := s[2:]
		if suffix == "" {
			return "", fmt.Errorf("%w: wildcard domain must be *.example.com form", ErrInvalidDomains)
		}
		converted, err := idna.Lookup.ToASCII(suffix)
		if err != nil {
			return "", fmt.Errorf("%w: %q is not a valid domain name", ErrInvalidDomains, raw)
		}
		ascii = "*." + converted
	} else {
		converted, err := idna.Lookup.ToASCII(s)
		if err != nil {
			return "", fmt.Errorf("%w: %q is not a valid domain name", ErrInvalidDomains, raw)
		}
		ascii = converted
	}
	if err := validateDomainSyntax(ascii); err != nil {
		return "", err
	}
	return ascii, nil
}

func validateDomainSyntax(domain string) error {
	if domain == "" || len(domain) > 253 {
		return fmt.Errorf("%w: %q", ErrInvalidDomains, domain)
	}
	if strings.Contains(domain, "..") {
		return fmt.Errorf("%w: %q contains empty label", ErrInvalidDomains, domain)
	}
	if strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return fmt.Errorf("%w: %q has invalid dot placement", ErrInvalidDomains, domain)
	}

	isWildcard := strings.HasPrefix(domain, "*.")
	if isWildcard {
		if domain == "*." || domain == "*" {
			return fmt.Errorf("%w: wildcard domain must be *.example.com form", ErrInvalidDomains)
		}
		if strings.Contains(domain[2:], "*") {
			return fmt.Errorf("%w: wildcard is only allowed in the leftmost label", ErrInvalidDomains)
		}
		domain = domain[2:]
		if domain == "" {
			return fmt.Errorf("%w: wildcard domain must include a suffix", ErrInvalidDomains)
		}
	} else if strings.Contains(domain, "*") {
		return fmt.Errorf("%w: wildcard is only allowed in the leftmost label", ErrInvalidDomains)
	}

	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return fmt.Errorf("%w: %q must contain at least two labels", ErrInvalidDomains, domain)
	}
	for _, label := range labels {
		if err := validateDomainLabel(label); err != nil {
			return err
		}
	}
	return nil
}

func validateDomainLabel(label string) error {
	if label == "" || len(label) > 63 {
		return fmt.Errorf("%w: invalid label %q", ErrInvalidDomains, label)
	}
	if label[0] == '-' || label[len(label)-1] == '-' {
		return fmt.Errorf("%w: label %q cannot start or end with hyphen", ErrInvalidDomains, label)
	}
	for i := 0; i < len(label); i++ {
		c := label[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			continue
		}
		return fmt.Errorf("%w: label %q contains invalid character", ErrInvalidDomains, label)
	}
	if !utf8.ValidString(label) {
		return fmt.Errorf("%w: label %q is not valid UTF-8", ErrInvalidDomains, label)
	}
	return nil
}

func normalizeKeyType(raw string) (string, error) {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "", "ec_p256", "ec-p256", "ec p-256":
		return store.CertKeyTypeECP256, nil
	case "rsa_2048", "rsa-2048", "rsa 2048":
		return store.CertKeyTypeRSA2048, nil
	default:
		return "", ErrInvalidKeyType
	}
}
