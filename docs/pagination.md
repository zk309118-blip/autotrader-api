# Pagination and partitioning

## The ceiling

`totalResultCount` is honest. A national search reports 3.4 million matches.
What you can retrieve from a single query is not:

| Limit | Value | Behaviour when exceeded |
| --- | --- | --- |
| `numRecords` | 100 | silently clamps |
| `firstRecord` | 300 | returns `listings: []` with no `totalResultCount` |

So **400 records maximum per query**, however many match. Measured by bisection:
`firstRecord=300` returns data, `firstRecord=301` returns an empty array. Not
an error — an empty success.

To collect more, split the query into shards that each fall under 400.

## Choosing a partition axis

A partition only works if the shards cover the space exactly. Some filters
quietly drop rows, which makes the union incomplete without any error.

Measured against an unfiltered control (`makeCode=ASTON`, national, 1189
listings):

| Axis | Sum of parts | Control | Exact? |
| --- | --- | --- | --- |
| `modelCode` (17 models) | 1189 | 1189 | ✅ |
| `listingType` (4 values) | 1189 | 1189 | ✅ |
| `sellerType` (`d`+`p`) | 1190 | 1189 | ✅ (drift during sweep) |
| year range | exact when the range spans all model years | | ✅ |
| `minPrice`/`maxPrice` | 703 | 749 (used only) | ❌ |

### Why price fails

Applying *any* price bound excludes listings whose price is null — the ones
carrying `noPriceLabel: "Contact Dealer For Price"`. Even a bound that should
be a no-op:

```
ASTON USED national                     749
  + minPrice=0                          703
  + maxPrice=25000000                   703
ASTON NEW  national                     440
  + maxPrice=25000000                   421
```

Roughly 5% of inventory, dropped without a warning. Price is the most natural
axis to reach for and the one that will silently corrupt a dataset.

Year ranges do not have this problem:

```
ASTON NEW national                      440
  + startYear=1980&endYear=2030         440
```

### A caution on year bounds

Year is only lossless if the range spans every model year present. Sweeping
2000–2028 on the same data returned 1184 of 1189 — the missing five were older
classics. Start at 1900 and end past the current model year.

## Recommended descent

Descend only when a shard overflows, and stay on lossless axes:

```
makeCode
└─ modelCode
   └─ listingType
      └─ year range (bisect)
         └─ sellerType
            └─ price bisection  ← last resort, warn loudly
```

Each level is checked with a cheap `numRecords=1` probe that reads
`totalResultCount`; if it fits under 400 the shard is drained in four pages of
100, otherwise it splits. Dedupe on `listing.id`.

[`examples/sharding`](../examples/sharding) implements exactly this:

```console
$ go run ./examples/sharding --make ASTON --radius 0
ASTON: 1189 listings reported

  ASTONDB11                                    +58   running=58
  ASTONDB12                                    +117  running=175
  ASTONDBX/USED MY1900-2035                    +211  running=386
  ...
  VANTAGE                                      +312  running=1187
  VIRAGE                                       +2    running=1189

collected 1189 of 1189 reported (100.0%) in 47 queries, 12.804s
```

Note the shape: 1189 listings retrieved with 47 requests, against a per-query
ceiling of 400. Two models needed a condition split; everything else fit whole.

## Scale

For a full national sweep, the arithmetic is roughly:

- 3.4M listings, 400 reachable per query → ~8,500 queries at perfect packing
- realistically 20,000–35,000 queries with imperfect shards
- at the observed unthrottled rate, well under an hour of wall clock
- ~16 KB of raw JSON per listing, so ~54 GB if you keep everything, far less if
  you drop the image arrays

Always verify a sweep against a control count before trusting it. The failure
mode of a bad partition is a plausible-looking dataset that is quietly missing
5% of its rows.
