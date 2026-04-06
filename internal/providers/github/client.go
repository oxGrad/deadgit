package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"time"
)

const (
	maxRetries     = 3
	baseBackoff    = 500 * time.Millisecond
	requestTimeout = 30 * time.Second
)

type client struct {
	http    *http.Client
	authHdr string
	baseURL string
}

func newClient(baseURL, pat string) *client {
	return &client{
		http:    &http.Client{Timeout: requestTimeout},
		authHdr: "Bearer " + pat,
		baseURL: baseURL,
	}
}

func (c *client) get(url string, out any) error {
	body, err := c.getRaw(url)
	if err != nil {
		return err
	}
	return json.NewDecoder(bytes.NewReader(body)).Decode(out)
}

func (c *client) getRaw(url string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(math.Pow(2, float64(attempt-1))) * baseBackoff)
		}
		body, retry, err := c.doRequest(url)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, lastErr
}

func (c *client) doRequest(url string) ([]byte, bool, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Authorization", c.authHdr)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == 403 {
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, _ := strconv.Atoi(ra); secs > 0 {
				time.Sleep(time.Duration(secs) * time.Second)
			}
		}
		return nil, true, fmt.Errorf("HTTP %d: rate limited", resp.StatusCode)
	}
	if resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("HTTP %d: server error", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("HTTP %d: %s", resp.StatusCode, url)
	}
	body, err := io.ReadAll(resp.Body)
	return body, false, err
}
