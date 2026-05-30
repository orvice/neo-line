package probe

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/orvice/neo-line/internal/store"
)

// probeURL issues an HTTP/HTTPS request and validates the response status code.
// For https targets it also performs the TLS handshake and records peer
// certificate metadata.
func probeURL(ctx context.Context, m store.Monitor, timeout time.Duration) outcome {
	target := strings.TrimSpace(m.URL)
	if target == "" {
		return outcome{status: store.StatusDown, stage: StageHTTP, errMsg: "monitor has no url configured"}
	}
	parsed, err := url.Parse(target)
	if err != nil {
		return outcome{status: store.StatusDown, stage: StageHTTP, errMsg: fmt.Sprintf("invalid url: %v", err)}
	}
	isHTTPS := strings.EqualFold(parsed.Scheme, "https")

	method := m.Method
	if method == "" {
		method = http.MethodGet
	}

	tlsConfig := &tls.Config{InsecureSkipVerify: isHTTPS && !m.TLSVerify}
	if m.SNIName != "" {
		tlsConfig.ServerName = m.SNIName
	}
	transport := &http.Transport{
		TLSClientConfig:   tlsConfig,
		DisableKeepAlives: true,
		Proxy:             http.ProxyFromEnvironment,
	}
	client := &http.Client{Transport: transport, Timeout: timeout}

	req, err := http.NewRequestWithContext(ctx, method, target, nil)
	if err != nil {
		return outcome{status: store.StatusDown, stage: StageHTTP, errMsg: err.Error()}
	}
	for k, v := range m.Headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		stage, msg := classifyHTTPError(err, isHTTPS)
		return outcome{status: store.StatusDown, stage: stage, errMsg: msg}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	out := outcome{
		httpCode: uint32(resp.StatusCode),
		port:     m.Port,
	}
	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		out.certificate = certificateInfo(resp.TLS.PeerCertificates[0])
	}

	if !statusCodeAccepted(uint32(resp.StatusCode), m.ExpectedStatusCodes) {
		out.status = store.StatusDown
		out.stage = StageHTTP
		out.errMsg = fmt.Sprintf("unexpected status code %d", resp.StatusCode)
		return out
	}

	out.status = store.StatusHealthy
	out.stage = StageNone
	return out
}

// statusCodeAccepted reports whether code matches the expected expression.
// The expression is a comma-separated list of single codes and inclusive
// ranges, for example "200-299,301,302". An empty expression accepts only 200.
func statusCodeAccepted(code uint32, expected string) bool {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return code == http.StatusOK
	}
	for part := range strings.SplitSeq(expected, ",") {
		if lo, hi, ok := parseStatusCodeRange(part); ok && code >= lo && code <= hi {
			return true
		}
	}
	return false
}

// parseStatusCodeRange parses a single expression segment into an inclusive
// [lo, hi] range. A bare code like "200" yields lo == hi. Malformed segments
// return ok == false and are ignored by the caller.
func parseStatusCodeRange(part string) (lo, hi uint32, ok bool) {
	part = strings.TrimSpace(part)
	if part == "" {
		return 0, 0, false
	}
	if before, after, found := strings.Cut(part, "-"); found {
		low, err1 := strconv.ParseUint(strings.TrimSpace(before), 10, 32)
		high, err2 := strconv.ParseUint(strings.TrimSpace(after), 10, 32)
		if err1 != nil || err2 != nil || low > high {
			return 0, 0, false
		}
		return uint32(low), uint32(high), true
	}
	v, err := strconv.ParseUint(part, 10, 32)
	if err != nil {
		return 0, 0, false
	}
	return uint32(v), uint32(v), true
}

// classifyHTTPError refines a request error, attributing TLS failures to the
// TLS stage when the target uses https.
func classifyHTTPError(err error, isHTTPS bool) (stage, message string) {
	stage, message = classifyStage(err)
	if isHTTPS {
		lower := strings.ToLower(message)
		if strings.Contains(lower, "tls") || strings.Contains(lower, "certificate") || strings.Contains(lower, "x509") || strings.Contains(lower, "handshake") {
			return StageTLS, message
		}
	}
	return stage, message
}

var _ = context.Background
