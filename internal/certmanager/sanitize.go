package certmanager

import (
	"strings"
)

const maxCertificateErrorSummaryLength = 500

var sensitiveCertificateErrorMarkers = []string{
	"token",
	"pem",
	"eab",
	"external account binding",
	"hmac",
	"api key",
	"private key",
	"account key",
	"authorization:",
	"bearer ",
	"order url",
	"new-order",
	"/order/",
	"acme-staging",
	"directory",
}

func sanitizeCertificateError(err error, fallback string, secretValues ...string) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	for _, marker := range sensitiveCertificateErrorMarkers {
		if strings.Contains(lower, marker) {
			return fallback
		}
	}
	for _, secret := range secretValues {
		secret = strings.TrimSpace(secret)
		if secret != "" && strings.Contains(msg, secret) {
			return fallback
		}
	}
	if len(msg) > maxCertificateErrorSummaryLength {
		msg = msg[:maxCertificateErrorSummaryLength]
	}
	return msg
}

func sanitizeIssueError(err error) string {
	return sanitizeCertificateError(err, "certificate issuance failed")
}

func sanitizeRegistrationError(err error, secretValues ...string) string {
	return sanitizeCertificateError(err, "certificate issuer registration failed", secretValues...)
}
