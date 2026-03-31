package azuredevops

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

const (
	maxRetries = 3
	baseDelay  = 500 * time.Millisecond
)

// Client is an authenticated Azure DevOps HTTP client.
type Client struct {
	pat        string
	httpClient *http.Client
}

// NewClient creates a Client with a 30-second timeout.
func NewClient(pat string) *Client {
	return &Client{
		pat: pat,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Get performs a GET request, decoding the JSON response into dest.
// Retries up to maxRetries times on 429 and 5xx responses.
func (c *Client) Get(url string, dest interface{}) error {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			delay := baseDelay * time.Duration(1<<uint(attempt-1))
			slog.Debug("retrying request", "url", url, "attempt", attempt+1, "delay", delay)
			time.Sleep(delay)
		}

		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		req.SetBasicAuth("", c.pat)
		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			retryAfter := resp.Header.Get("Retry-After")
			resp.Body.Close()
			if retryAfter != "" {
				if secs, err := strconv.Atoi(retryAfter); err == nil && secs > 0 {
					time.Sleep(time.Duration(secs) * time.Second)
					continue
				}
			}
			lastErr = fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return fmt.Errorf("HTTP %d from %s: %s", resp.StatusCode, url, string(body))
		}

		defer resp.Body.Close()
		if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
			return fmt.Errorf("decode response from %s: %w", url, err)
		}
		return nil
	}
	return fmt.Errorf("max retries exceeded for %s: %w", url, lastErr)
}

// GetRaw performs a GET request and returns the raw body bytes.
// Applies the same retry logic as Get.
func (c *Client) GetRaw(url string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			delay := baseDelay * time.Duration(1<<uint(attempt-1))
			time.Sleep(delay)
		}

		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.SetBasicAuth("", c.pat)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			resp.Body.Close()
			lastErr = fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
			continue
		}

		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read body from %s: %w", url, err)
		}
		return body, nil
	}
	return nil, fmt.Errorf("max retries exceeded for %s: %w", url, lastErr)
}
