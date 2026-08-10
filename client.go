// Package autotrader is a client for autotrader.com's internal listing API.
//
// The API needs no key, token, cookie, or session warmup. Access is gated
// purely on TLS/HTTP2 client fingerprint, and Go's standard library passes,
// which is why this package has zero dependencies. Do not swap in an exotic
// HTTP transport without reading docs/bot-protection.md -- and note that a
// datacenter IP is blocked regardless of transport.
package autotrader

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultBaseURL is the API origin.
const DefaultBaseURL = "https://www.autotrader.com"

const (
	searchPath   = "/rest/lsc/listing"
	codebookPath = "/collections/bonnet-reference/searchoptions"
)

// Client talks to the listing API. The zero value is not usable; call New.
type Client struct {
	HTTPClient *http.Client
	BaseURL    string
	UserAgent  string
	Referer    string
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient supplies a custom *http.Client.
//
// Be careful: the bot gate keys on TLS fingerprint. Go's default transport
// passes. Forcing HTTP/2-only, or wiring in a third-party TLS stack, can get
// you blocked. Setting a proxy or timeout is safe.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.HTTPClient = h }
}

// WithBaseURL overrides the API origin, mainly for tests.
func WithBaseURL(u string) Option {
	return func(c *Client) { c.BaseURL = strings.TrimSuffix(u, "/") }
}

// WithUserAgent sets the User-Agent header. The API does not require a
// browser-like value -- requests with no User-Agent at all succeed.
func WithUserAgent(ua string) Option {
	return func(c *Client) { c.UserAgent = ua }
}

// New returns a Client with sensible defaults.
func New(opts ...Option) *Client {
	c := &Client{
		HTTPClient: &http.Client{Timeout: 45 * time.Second},
		BaseURL:    DefaultBaseURL,
		UserAgent:  "autotrader-api-go/1.0 (+https://github.com/dhruvkar/autotrader-api)",
		Referer:    DefaultBaseURL + "/cars-for-sale/all-cars",
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Search runs a listing query.
//
// A single query can return at most MaxReachable records no matter how large
// TotalResultCount is. To collect more, partition the query -- see
// examples/sharding.
func (c *Client) Search(ctx context.Context, p SearchParams) (*SearchResult, error) {
	body, err := c.get(ctx, searchPath, p.Values())
	if err != nil {
		return nil, err
	}
	var out SearchResult
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("autotrader: decoding search response: %w", err)
	}
	return &out, nil
}

// SearchRaw returns the undecoded JSON, for fields this package does not model
// (facet counts, KBB blocks, incentives).
func (c *Client) SearchRaw(ctx context.Context, p SearchParams) ([]byte, error) {
	return c.get(ctx, searchPath, p.Values())
}

// ByVIN looks up a single vehicle. Returns nil when nothing matches.
func (c *Client) ByVIN(ctx context.Context, vin string) (*Listing, error) {
	res, err := c.Search(ctx, SearchParams{VIN: vin, NumRecords: 1})
	if err != nil {
		return nil, err
	}
	if len(res.Listings) == 0 {
		return nil, nil
	}
	return &res.Listings[0], nil
}

// ByListingID looks up a single vehicle by its numeric listing id.
func (c *Client) ByListingID(ctx context.Context, id string) (*Listing, error) {
	res, err := c.Search(ctx, SearchParams{ListingID: id, NumRecords: 1})
	if err != nil {
		return nil, err
	}
	if len(res.Listings) == 0 {
		return nil, nil
	}
	return &res.Listings[0], nil
}

// get performs the request and converts a bot-protection interstitial into
// ErrBlocked. This check is the reason to route every call through here: the
// block arrives as HTTP 200 with an HTML body, so status alone proves nothing.
func (c *Client) get(ctx context.Context, path string, q url.Values) ([]byte, error) {
	u := c.BaseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "*/*")
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	if c.Referer != "" {
		req.Header.Set("Referer", c.Referer)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("autotrader: %s: %w", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("autotrader: reading %s: %w", path, err)
	}

	if isBlocked(resp, body) {
		return nil, ErrBlocked
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{StatusCode: resp.StatusCode, Path: path, Body: string(body)}
	}
	return body, nil
}

// isBlocked detects the interstitial. A genuine response is JSON; the block is
// an HTML document served with a 200.
func isBlocked(resp *http.Response, body []byte) bool {
	if strings.Contains(resp.Header.Get("Content-Type"), "json") {
		return false
	}
	trimmed := strings.TrimLeft(string(body), " \t\r\n\ufeff")
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return false
	}
	return true
}
