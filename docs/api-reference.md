# API reference

Origin: `https://www.autotrader.com`

Everything here is unauthenticated. Verified 2026-08-10.

## Endpoints

### `GET /rest/lsc/listing`

The search endpoint backing the results page. Returns listings, the dealers
that own them, facet counts, and a match total.

A mirror at `/collections/lcServices/rest/lsc/listing` serves the homepage
modules. Same data; it nests facets under `filterGroups` instead of `filters`.

### `GET /collections/bonnet-reference/searchoptions`

The codebook: every valid filter value, including all 72 makes with their model
and trim trees. ~420 KB. A snapshot is vendored at `data/searchoptions.json`.

### Others seen in traffic

| Path | Purpose |
| --- | --- |
| `/collections/bonnet-reference/markets?zip=` | ZIP → DMA/market resolution |
| `/collections/ccServices/rest/ccs/models?makeCode=` | models for a make |
| `/rest/lsc/modelinfo/ymm` | year/make/model aggregates |
| `/rest/lsc/crawl/stats/mileage` | mileage distribution stats |
| `/cars-for-sale/kbbresearch/reference/vehicles` | KBB trim and pricing joins |
| `/rest/retailing/budget` | monthly-payment calculator |

There is **no separate vehicle-detail endpoint**. `/rest/lsc/listing/{id}` and
every variant returns 404. Single-vehicle lookups go through the search
endpoint via `listingId` or `vin`.

## Search parameters

Unknown parameters are ignored rather than rejected, so a misspelling returns
unfiltered results. Always confirm `totalResultCount` moved after adding a
filter.

### Location

| Parameter | Notes |
| --- | --- |
| `zip` | origin ZIP code |
| `searchRadius` | `10 25 50 75 100 200 300 400 500 0` — **`0` means national** |
| `marketExtension` | `include` widens beyond the radius to nearby markets |

### Vehicle

| Parameter | Notes |
| --- | --- |
| `makeCode` | repeatable; OR'd. e.g. `TOYOTA` |
| `modelCode` | repeatable. e.g. `TACOMA` |
| `trimCode` | repeatable |
| `listingType` | `NEW` `USED` `CERTIFIED` `3P_CERT` |
| `sellerType` | `d` dealer, `p` private party |
| `startYear` / `endYear` | integers |
| `minPrice` / `maxPrice` | integers — **lossy, see below** |
| `mileage` | band code such as `-60000`, not a raw number |
| `driveGroup` | `AWD4WD` `FWD` `RWD` |
| `fuelTypeGroup` | `GSL` `DSL` `HYB` `ELE` `HYD` `PIH` |
| `transmissionCode` | `AUT` `MAN` |
| `vehicleStyleCode` | `CONVERT` `COUPE` `HATCH` `SEDAN` `SUVCROSS` `TRUCKS` `VANMV` `WAGON` |
| `extColorSimple` / `intColorSimple` | `RED` `BLACK` … (19 values) |
| `vehicleHistoryType` | `NO_ACCIDENTS` `ONE_OWNER` `CLEAN_TITLE` `NO_FRAME_DAMAGE` `FREE_REPORT` |
| `featureCode` | numeric codes, e.g. `1062` = Keyless Entry (34 values) |
| `engineCode` | `3CLDR` … `12CLDR` |
| `dealType` | `goodprice` `greatprice` |
| `maximumSeats`, `doorCode` | single values |

### Lookup

| Parameter | Notes |
| --- | --- |
| `vin` | returns the single matching vehicle |
| `listingId` | numeric listing id |

### Paging and sort

| Parameter | Notes |
| --- | --- |
| `numRecords` | clamps to 100 |
| `firstRecord` | max 300; beyond that returns an empty array |
| `sortBy` | `relevance` `derivedpriceASC` `derivedpriceDESC` `mileageASC` `yearDESC` `distanceASC` `datelistedDESC` |

### The price parameters are lossy

`minPrice` / `maxPrice` exclude every listing with a null price. This applies
even to a nominally no-op bound:

```
ASTON USED national, no price filter   749
ASTON USED national, minPrice=0        703
ASTON USED national, maxPrice=25000000 703
```

Never use price to partition a crawl. See [pagination.md](pagination.md).

## Response shape

```jsonc
{
  "listings": [ ... ],
  "owners":   [ ... ],   // dealers referenced by listing.ownerId
  "filters":  { ... },   // facet counts, useful for discovering valid values
  "totalResultCount": 40420,
  "searchRadius": 50,
  "zip": "63101"
}
```

### Listing fields

| Field | Notes |
| --- | --- |
| `id` | numeric listing id, stable; use for dedup |
| `vin` | |
| `year`, `title`, `titleLong` | `title` is a plain string |
| `make`, `model`, `trim` | `{code, name}` |
| `listingType` | `NEW` / `USED` / … |
| `mileage` | `{label, value}` — **value is a display string, `"26,648"`** |
| `pricingDetail` | see below |
| `daysOnSite` | useful for freshness and days-on-market |
| `ownerId` | join key into `owners[]` |
| `images.sources[]` | often 20+ photos, `{src, alt, width, height}` |
| `color`, `engine`, `fuelType`, `transmission`, `driveType`, `bodyStyles` | |
| `specifications` | label/value pairs mirroring the above |

### `pricingDetail`

| Field | Notes |
| --- | --- |
| `salePrice` | **null on ~30% of NEW listings** |
| `displayPrice` | usually present when `salePrice` is not |
| `msrp` | |
| `noPriceLabel` | e.g. `"Contact Dealer For Price"` |
| `dealerFeesTotal`, `preFeeDerivedPrice`, `incentive` | |
| `dealIndicator` | `Great` / `Good` |
| `kbbFppAmount`, `kbbFppDelta` | KBB fair purchase price and delta |

Read prices through a fallback chain (`salePrice` → `displayPrice` → `msrp`),
which is what `PricingDetail.Price()` does.

### Owner fields

Dealers arrive fully populated — this is a usable dealer directory in its own
right:

| Field | Notes |
| --- | --- |
| `id`, `name` | |
| `location.address` | street, city, state, ZIP, latitude, longitude |
| `phone.value` | 10-digit string |
| `website.href` | dealer's own site |
| `dealerFinancingHref` | financing application URL |
| `rating` | `{value, count}` |
| `hours[]` | per-day open/close |
| `distanceFromSearch` | miles from the search origin |
