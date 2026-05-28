package probe

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

	if !statusCodeAccepted(uint32(resp.StatusCode), m.ExpectedStatusCode) {
		out.status = store.StatusDown
		out.stage = StageHTTP
		out.errMsg = fmt.Sprintf("unexpected status code %d", resp.StatusCode)
		return out
	}

	out.status = store.StatusHealthy
	out.stage = StageNone
	return out
}

func statusCodeAccepted(code uint32, expected []uint32) bool {
	if len(expected) == 0 {
		return code == http.StatusOK
	}
	for _, e := range expected {
		if e == code {
			return true
		}
	}
	return false
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
