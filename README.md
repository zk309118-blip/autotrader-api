
# autotrader-api

A Go client and CLI for autotrader.com's internal listing API.

No API key, no token, no cookies, no session warmup. Zero dependencies — the
standard library is all you need, for a reason explained below.

```
go install github.com/dhruvkar/autotrader-api/cmd/atc@latest
```

```console
$ atc search --make TOYOTA --model TACOMA --zip 63101 --radius 100 \
             --condition USED --mileage -60000 --sort derivedpriceASC --limit 8

YEAR  VEHICLE        MILES  PRICE   COND  DEALER                      VIN
2009  Toyota Tacoma  56025  $15120  USED  Bommarito Toyota            5TENX22N19Z624229
2016  Toyota Tacoma  50871  $21996  USED  Watermark Ford Hyundai of…  5TFSX5EN7GX044911
2013  Toyota Tacoma  50384  $24985  USED  Rusty Drewing Chevrolet G…  3TMLU4EN4DM124143
...
8 shown | 92 total matches
```

---

## Read this first

Two things will cost you a day if you find them the hard way.

### 1. A block looks exactly like a success

When the bot gate rejects you it returns **HTTP 200** with an HTML page titled
"Autotrader - page unavailable". Not 403. Not 429. Any client that trusts the
status code treats a block as an empty result set and produces silent garbage.

This package converts that into a typed [`ErrBlocked`](errors.go). If you write
your own client, replicate that check first.

### 2. Access is gated on TLS fingerprint, and cloud IPs are blocked

There is no auth to reverse-engineer. Whether you get data depends on what your
HTTP stack's TLS/HTTP2 handshake looks like, and where you're calling from.

| Client | Works |
| --- | --- |
| Go `net/http` (stdlib, no deps) | ✅ |
| `curl --http1.1` | ✅ |
| Python + `curl_cffi` (`impersonate="chrome"`) | ✅ |
| `curl` over HTTP/2 | ❌ |
| Python `requests` / `httpx` / `urllib3` | ❌ regardless of headers |

Headers are not the discriminator. Mimicking curl's exact header set does not
rescue Python; requests with no `User-Agent` at all succeed.

**It will not run from a cloud VM.** A DigitalOcean host was blocked on the
exact configuration that worked from a residential connection moments earlier.
Deploy it to a server and it will fail in a way that reads like a code bug. Run
it from a residential IP, or a residential proxy.

Full measurements in [docs/bot-protection.md](docs/bot-protection.md).

---

## Library

```go
package main

import (
    "context"
    "fmt"

    autotrader "github.com/dhruvkar/autotrader-api"
)

func main() {
    c := autotrader.New()

    res, err := c.Search(context.Background(), autotrader.SearchParams{
        Zip:          "63101",
        SearchRadius: 100,
        MakeCode:     []string{"TOYOTA"},
        ModelCode:    []string{"TACOMA"},
        ListingType:  []string{autotrader.ConditionUsed},
        MaxPrice:     35000,
        SortBy:       autotrader.SortPriceAsc,
        NumRecords:   50,
    })
    if err != nil {
        panic(err)
    }

    dealers := res.OwnerByID()
    for _, l := range res.Listings {
        price, _ := l.Pricing.Price()
        miles, _ := l.Miles()
        fmt.Printf("%d %s — %d mi — $%d — %s\n",
            l.Year, l.Title, miles, price, dealers[l.OwnerID].Name)
    }
}
```

Single-vehicle lookups work off the same endpoint:

```go
listing, err := c.ByVIN(ctx, "5TFSX5EN7GX044911")
listing, err := c.ByListingID(ctx, "784528754")
```

`SearchRaw` returns undecoded JSON for the fields this package doesn't model —
facet counts, KBB valuation blocks, incentives, dealer disclaimers.

---

## The 400-record ceiling

`TotalResultCount` is honest — a national search reports millions. What you can
*retrieve* is capped:

- `numRecords` clamps to **100**
- `firstRecord` above **300** returns an empty array with no error
- so **400 records maximum per query**, regardless of matches

To go beyond that you partition the query. Which axis you pick matters more
than it looks, because some filters quietly drop rows:

| Partition axis | Sum of parts vs. control | Safe? |
| --- | --- | --- |
| `modelCode` | 1189 = 1189 | ✅ exact |
| `listingType` | 1189 = 1189 | ✅ exact |
| `sellerType` | 1190 vs 1189 | ✅ (inventory drift) |
| year range | exact when the range spans all model years | ✅ |
| `minPrice` / `maxPrice` | **703 vs 749** | ❌ **lossy** |

**Price is not a safe partition axis.** Applying any price bound — including a
nominally no-op `minPrice=0` — excludes every listing whose price is null
("Contact Dealer For Price"). That's ~5% of inventory, dropped silently. It's
the most natural axis to reach for and the one that will corrupt your dataset.

[`examples/sharding`](examples/sharding) implements the descent that avoids it
(model → condition → year → seller, price only as a warned last resort):

```console
$ go run ./examples/sharding --make ASTON --radius 0
ASTON: 1189 listings reported
  ...
collected 1189 of 1189 reported (100.0%) in 47 queries, 12.8s
```

More detail in [docs/pagination.md](docs/pagination.md).

---

## Codebook


Every valid filter value — 72 makes with their full model and trim trees, plus
colors, body styles, features, mileage bands — comes from one unauthenticated
reference call. A snapshot ships embedded, so this works offline:

```console
$ atc codebook                    # summary of every section
$ atc codebook --makes            # all 72 make codes
$ atc codebook --models TOYOTA    # model codes for a make
$ atc codebook --refresh > data/searchoptions.json   # re-vendor from live
```

---

## Traps worth knowing

- **Unknown parameters are ignored, not rejected.** `maxMileage=50000` does
  nothing at all; the real parameter is `mileage` and it takes a band code like
  `-60000`. A typo returns unfiltered results that look plausible. After adding
  a filter, confirm `TotalResultCount` actually moved.
- **`mileage.value` is a display string** — `"26,648"`, not a number. Use
  `Listing.Miles()`.
- **`salePrice` is null on ~30% of new listings.** Use `Pricing.Price()`, which
  falls back through `displayPrice` and `msrp`.
- **`searchRadius=0` means national**, not zero miles.
- Rate limiting was not observed at up to 36 req/s over 300 requests, but that
  is a burst, not a soak. Be considerate; add a delay.

---

## Reference

- [docs/api-reference.md](docs/api-reference.md) — endpoints, parameters, response shape
- [docs/bot-protection.md](docs/bot-protection.md) — fingerprint measurements
- [docs/pagination.md](docs/pagination.md) — the ceiling and partition strategy

Findings verified 2026-08-10 against the live site.

## Disclaimer

Provided for reference and educational use. Automated collection is contrary to
autotrader.com's terms of service, and the data belongs to Cox Automotive and
its dealers. You are responsible for how you use this. No listing data is
distributed with this repository.

## License

MIT — see [LICENSE](LICENSE).
