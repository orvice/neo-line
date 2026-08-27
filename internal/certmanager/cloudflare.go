package certmanager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const defaultCloudflareAPIBase = "https://api.cloudflare.com/client/v4"

// CloudflareClient verifies Cloudflare API tokens via the user/tokens/verify
// endpoint.
type CloudflareClient struct {
	baseURL string
	client  *http.Client
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
	token = strings.TrimSpace(token)
	if token == "" {
		return ErrCloudflareTokenInvalid
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/user/tokens/verify", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	res, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("cloudflare verify request: %w", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read cloudflare verify response: %w", err)
	}

	var parsed cloudflareVerifyResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fmt.Errorf("parse cloudflare verify response: %w", err)
	}
	if res.StatusCode == http.StatusOK && parsed.Success {
		return nil
	}
	msg := cloudflareErrorMessage(parsed)
	if msg == "" {
		msg = "cloudflare token verification failed"
	}
	return fmt.Errorf("%w: %s", ErrCloudflareTokenInvalid, msg)
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
