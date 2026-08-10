package autotrader

import (
	"errors"
	"fmt"
)

// ErrBlocked is returned when Akamai Bot Manager rejects the request.
//
// This is the single most important error in this package. The block does NOT
// arrive as 403 or 429 -- it arrives as HTTP 200 with an HTML interstitial
// titled "Autotrader - page unavailable". A client that only checks the status
// code will treat a block as a successful empty result and silently produce
// garbage.
//
// If you see this, the cause is almost always one of:
//
//   - You are running from a cloud/datacenter IP. DigitalOcean is blocked
//     outright; residential IPs pass. See docs/bot-protection.md.
//   - You replaced the HTTP transport with one whose TLS fingerprint is
//     flagged (anything built on Python's ssl module, or curl over HTTP/2).
var ErrBlocked = errors.New("autotrader: request blocked by bot protection (HTTP 200 + HTML interstitial)")

// APIError is a non-2xx response from the API.
type APIError struct {
	StatusCode int
	Path       string
	Body       string
}

func (e *APIError) Error() string {
	body := e.Body
	if len(body) > 200 {
		body = body[:200] + "..."
	}
	return fmt.Sprintf("autotrader: %s returned %d: %s", e.Path, e.StatusCode, body)
}
