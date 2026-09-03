
# Bot protection

The site runs Akamai Bot Manager. There is no API key, token, cookie, or
session to acquire — a cold request with no cookie jar and no warmup succeeds.
Access is decided by your TLS/HTTP2 client fingerprint and your IP reputation.

All measurements below were taken 2026-08-10 against
`GET /rest/lsc/listing`, 6 trials each, from a residential connection unless
stated otherwise.

## The block is an HTTP 200

The rejection is not a 403 or a 429. It is:

```
HTTP/1.1 200 OK
Content-Type: text/html

<!DOCTYPE html>
<html ...>
<head><title>Autotrader - page unavailable</title></head>
```

3,762 bytes of HTML with a success status. Detect it by content type or body
prefix, never by status code. This package does that in `isBlocked` and returns
`ErrBlocked`.

## Which clients pass

| Client | Transport | Result |
| --- | --- | --- |
| Go `net/http` | h2 (negotiated) | 6/6 ✅ |
| `curl --http1.1` | h1.1 | 6/6 ✅ |
| `curl --http1.1`, no User-Agent | h1.1 | 6/6 ✅ |
| `curl_cffi` `impersonate="chrome"` | h2 | 6/6 ✅ |
| `curl` default / `--http2` | h2 | 0/6 ❌ |
| Python `requests` | h1.1 | 0/6 ❌ |
| Python `httpx` | h1.1 | 0/6 ❌ |

Results are deterministic, not probabilistic — every client scored 0/6 or 6/6.

### It is not the headers

Python `requests` was retried with the User-Agent removed, with
`Accept-Encoding` suppressed, with `Accept-Encoding: identity`, and with curl's
exact header set. All blocked. Meanwhile bare `curl --http1.1` with no headers
at all succeeds. The discriminator is the TLS ClientHello, plus an HTTP/2
settings fingerprint that curl's h2 fails and Go's passes.

The practical consequence: **Go needs no special dependency, and Python needs
`curl_cffi` permanently.** That is why this package is stdlib-only.

## IP reputation

| Host | IP type | Result |
| --- | --- | --- |
| Residential (Pop!\_OS desktop) | residential | 6/6 ✅ |
| Residential (macOS laptop) | residential | 6/6 ✅ |
| DigitalOcean droplet | datacenter | **0/6 ❌** |

The DigitalOcean test used `curl --http1.1` — the identical invocation that
scored 6/6 residentially, run within minutes of the control. A simultaneous
control from the residential host confirmed the site was not globally blocking
at that moment.

**Datacenter IPs are blocked outright.** No transport choice fixes this. If you
need to run this from infrastructure, you need residential proxies.

## Rate limiting

A ramp from a residential IP, varying ZIP and offset per request to defeat
caching:

| Phase | Requests | Concurrency | Achieved rate | Failures |
| --- | --- | --- | --- | --- |
| A | 60 | 1 | 1.1 req/s | 0 |
| B | 60 | 4 | 7.9 req/s | 0 |
| C | 80 | 10 | 21.5 req/s | 0 |
| D | 100 | 20 | 36.3 req/s | 0 |

300 requests, no throttling, no challenge, no degradation.

Treat this as a burst measurement, not a soak test. Akamai may run
longer-window counters that a multi-hour crawl would trip and a 90-second ramp
would not. Add a delay between requests.

## The sensor script

For completeness: the page loads an obfuscated Akamai sensor from a randomized
same-origin path (`/Tkz1MdJqD9OV7lpSt_gmsJtjKgY/...`) and POSTs telemetry back
as `{"body": "<obfuscated>"}`. The script defines the `bmak` global, and the
site's own cookie inventory lists `_abck`, `bm_sz`, and `ak_bmsc`.

None of that is required for API access. The sensor exists to *upgrade* a
session's trust; the API simply doesn't demand the upgraded cookie.
