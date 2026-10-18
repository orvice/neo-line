package certmanager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const defaultCloudflareAPIBase = "https://api.cloudflare.com/client/v4"

// CloudflareClient verifies Cloudflare API tokens via the user/tokens/verify
// endpoint.
type CloudflareClient struct {
	baseURL string
	client  *http.Client
	logger  *slog.Logger
}

func NewCloudflareClient(client *http.Client) *CloudflareClient {
	if client == nil {
		client = http.DefaultClient
	}
	return &CloudflareClient{baseURL: defaultCloudflareAPIBase, client: client}
}

type cloudflareVerifyResponse struct {
	Success bool `json:"success"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
	Messages []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"messages"`
}

// ErrCloudflareTokenInvalid is returned when Cloudflare rejects the token.
var ErrCloudflareTokenInvalid = errors.New("cloudflare api token verification failed")

func (c *CloudflareClient) VerifyCloudflareToken(ctx context.Context, token string) error {
	started := time.Now()
	token = strings.TrimSpace(token)
	if token == "" {
		err := ErrCloudflareTokenInvalid
		c.logTokenVerification(ctx, 0, false, nil, time.Since(started), err)
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/user/tokens/verify", nil)
	if err != nil {
		c.logTokenVerification(ctx, 0, false, nil, time.Since(started), err)
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	res, err := c.client.Do(req)
	if err != nil {
		err = fmt.Errorf("cloudflare verify request: %w", err)
		c.logTokenVerification(ctx, 0, false, nil, time.Since(started), err)
		return err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		err = fmt.Errorf("read cloudflare verify response: %w", err)
		c.logTokenVerification(ctx, res.StatusCode, false, nil, time.Since(started), err)
		return err
	}

	var parsed cloudflareVerifyResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		err = fmt.Errorf("parse cloudflare verify response: %w", err)
		c.logTokenVerification(ctx, res.StatusCode, false, nil, time.Since(started), err)
		return err
	}
	codes := cloudflareVerifyErrorCodes(parsed)
	if res.StatusCode == http.StatusOK && parsed.Success {
		c.logTokenVerification(ctx, res.StatusCode, true, codes, time.Since(started), nil)
		return nil
	}
	msg := cloudflareErrorMessage(parsed)
	if msg == "" {
		msg = "cloudflare token verification failed"
	}
	err = fmt.Errorf("%w: %s", ErrCloudflareTokenInvalid, msg)
	c.logTokenVerification(ctx, res.StatusCode, parsed.Success, codes, time.Since(started), err)
	return err
}

// SetLogger configures safe token verification diagnostics.
func (c *CloudflareClient) SetLogger(logger *slog.Logger) {
	if c != nil && logger != nil {
		c.logger = logger
	}
}

func (c *CloudflareClient) logTokenVerification(ctx context.Context, status int, success bool, codes []int, duration time.Duration, err error) {
	if c == nil || c.logger == nil {
		return
	}
	attrs := []any{
		"cloudflare_endpoint", "user_tokens_verify",
		"http_status", status,
		"success", success,
		"duration_ms", duration.Milliseconds(),
	}
	if len(codes) > 0 {
		attrs = append(attrs, "cloudflare_error_codes", codes)
	}
	if err != nil {
		attrs = append(attrs, safeErrorAttrs(err)...)
		c.logger.ErrorContext(ctx, "cloudflare token verification request failed", attrs...)
		return
	}
	c.logger.InfoContext(ctx, "cloudflare token verification request completed", attrs...)
}

func cloudflareVerifyErrorCodes(parsed cloudflareVerifyResponse) []int {
	codes := make([]int, 0, len(parsed.Errors)+len(parsed.Messages))
	for _, item := range parsed.Errors {
		if item.Code != 0 {
			codes = append(codes, item.Code)
		}
	}
	for _, item := range parsed.Messages {
		if item.Code != 0 {
			codes = append(codes, item.Code)
		}
	}
	return codes
}

func cloudflareErrorMessage(parsed cloudflareVerifyResponse) string {
	if len(parsed.Errors) > 0 && parsed.Errors[0].Message != "" {
		return parsed.Errors[0].Message
	}
	if len(parsed.Messages) > 0 && parsed.Messages[0].Message != "" {
		return parsed.Messages[0].Message
	}
	return ""
}
