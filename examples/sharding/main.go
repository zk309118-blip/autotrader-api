// Command sharding demonstrates how to collect more than 400 listings.
//
// A single query returns at most 400 records no matter how many match, so any
// broad search must be split into shards that each fall under that ceiling.
// The shards must partition the space *exactly*, or the union quietly loses
// rows.
//
// Which axes actually partition exactly, measured against an unfiltered
// control count:
//
//	modelCode     sum of parts == total    lossless
//	listingType   sum of parts == total    lossless
//	sellerType    sum of parts == total    lossless
//	year range    lossless if the range spans every model year present
//	minPrice/maxPrice                      LOSSY -- do not use
//
// The price trap is the important one. Applying any price bound at all,
// including a nominally no-op minPrice=0, drops every listing whose price is
// null ("Contact Dealer For Price"). On a national Aston Martin search that is
// 749 -> 703 used and 440 -> 421 new, roughly 5% of inventory, silently. This
// example therefore descends model -> condition -> year -> seller and only
// falls back to price as a last resort, with a loud warning.
//
// Usage:
//
//	go run ./examples/sharding --make ASTON --radius 0
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	autotrader "github.com/dhruvkar/autotrader-api"
)

// Model years span the whole site, including classics.
const (
	yearFloor   = 1900
	yearCeiling = 2035
)

var conditions = []string{
	autotrader.ConditionUsed,
	autotrader.ConditionNew,
	autotrader.ConditionCertified,
	autotrader.Condition3PCert,
}

type harvester struct {
	client  *autotrader.Client
	seen    map[int64]autotrader.Listing
	queries int
	pause   time.Duration
	lossy   int // shards that had to fall back to a lossy axis
}

func main() {
	makeCode := flag.String("make", "ASTON", "make code to harvest (see: atc codebook --makes)")
	zip := flag.String("zip", "63101", "origin ZIP")
	radius := flag.Int("radius", 0, "radius in miles; 0 is national")
	pause := flag.Duration("pause", 120*time.Millisecond, "delay between requests")
	flag.Parse()

	h := &harvester{
		client: autotrader.New(),
		seen:   map[int64]autotrader.Listing{},
		pause:  *pause,
	}
	ctx := context.Background()

	base := autotrader.SearchParams{
		Zip:          *zip,
		SearchRadius: *radius,
		MakeCode:     []string{*makeCode},
	}

	// Control count, taken before the sweep, to check completeness after.
	control, err := h.count(ctx, base)
	if err != nil {
		log.Fatalf("control count: %v", err)
	}
	fmt.Fprintf(os.Stderr, "%s: %d listings reported\n\n", *makeCode, control)

	cb, err := autotrader.EmbeddedCodebook()
	if err != nil {
		log.Fatal(err)
	}
	mk, ok := cb.FindMake(*makeCode)
	if !ok {
		log.Fatalf("unknown make %q", *makeCode)
	}

	start := time.Now()
	if err := h.byModel(ctx, base, mk); err != nil {
		log.Fatalf("harvest: %v", err)
	}

	fmt.Fprintf(os.Stderr, "\ncollected %d of %d reported (%.1f%%) in %d queries, %s\n",
		len(h.seen), control, 100*float64(len(h.seen))/float64(max(control, 1)),
		h.queries, time.Since(start).Round(time.Millisecond))
	if h.lossy > 0 {
		fmt.Fprintf(os.Stderr, "WARNING: %d shard(s) fell back to price bisection and may be incomplete\n", h.lossy)
	}
}

// byModel is the top lossless split. Descend only where a shard overflows.
func (h *harvester) byModel(ctx context.Context, p autotrader.SearchParams, mk autotrader.Make) error {
	if n, err := h.fits(ctx, p); err != nil {
		return err
	} else if n >= 0 {
		return h.drain(ctx, p, mk.Code)
	}

	for _, mo := range mk.Models {
		q := p
		q.ModelCode = []string{mo.Code}
		if err := h.byCondition(ctx, q, mo.Code); err != nil {
			return err
		}
	}
	return nil
}

func (h *harvester) byCondition(ctx context.Context, p autotrader.SearchParams, label string) error {
	if n, err := h.fits(ctx, p); err != nil {
		return err
	} else if n >= 0 {
		if n == 0 {
			return nil
		}
		return h.drain(ctx, p, label)
	}

	for _, c := range conditions {
		q := p
		q.ListingType = []string{c}
		if err := h.byYear(ctx, q, yearFloor, yearCeiling, label+"/"+c); err != nil {
			return err
		}
	}
	return nil
}

// byYear bisects the model-year range, which partitions exactly.
func (h *harvester) byYear(ctx context.Context, p autotrader.SearchParams, lo, hi int, label string) error {
	p.StartYear, p.EndYear = lo, hi

	n, err := h.fits(ctx, p)
	if err != nil {
		return err
	}
	if n == 0 {
		return nil
	}
	if n >= 0 {
		return h.drain(ctx, p, fmt.Sprintf("%s MY%d-%d", label, lo, hi))
	}
	if lo < hi {
		mid := lo + (hi-lo)/2
		if err := h.byYear(ctx, p, lo, mid, label); err != nil {
			return err
		}
		return h.byYear(ctx, p, mid+1, hi, label)
	}
	// A single model year still overflows: split by seller, also lossless.
	return h.bySeller(ctx, p, fmt.Sprintf("%s MY%d", label, lo))
}

func (h *harvester) bySeller(ctx context.Context, p autotrader.SearchParams, label string) error {
	for _, st := range []string{"d", "p"} {
		q := p
		q.SellerType = st

		n, err := h.fits(ctx, q)
		if err != nil {
			return err
		}
		if n == 0 {
			continue
		}
		if n >= 0 {
			if err := h.drain(ctx, q, label+"/"+st); err != nil {
				return err
			}
			continue
		}
		// Out of lossless axes. Price bisection can still reach most of the
		// shard, but will omit null-price listings -- so say so.
		h.lossy++
		fmt.Fprintf(os.Stderr, "  ! %s/%s exceeds the ceiling with no lossless axis left; "+
			"falling back to price (null-price listings will be missed)\n", label, st)
		if err := h.byPrice(ctx, q, 0, 25_000_000, label+"/"+st); err != nil {
			return err
		}
	}
	return nil
}

// byPrice is the lossy last resort. See the package comment.
func (h *harvester) byPrice(ctx context.Context, p autotrader.SearchParams, lo, hi int, label string) error {
	p.MinPrice, p.MaxPrice = lo, hi

	n, err := h.fits(ctx, p)
	if err != nil {
		return err
	}
	if n == 0 {
		return nil
	}
	if n >= 0 {
		return h.drain(ctx, p, fmt.Sprintf("%s $%d-%d", label, lo, hi))
	}
	if hi-lo <= 1 {
		return h.drain(ctx, p, fmt.Sprintf("%s $%d-%d (truncated)", label, lo, hi))
	}
	mid := lo + (hi-lo)/2
	if err := h.byPrice(ctx, p, lo, mid, label); err != nil {
		return err
	}
	return h.byPrice(ctx, p, mid+1, hi, label)
}

// fits reports the shard size when it is retrievable in one query, or -1 when
// it overflows and must be split further.
func (h *harvester) fits(ctx context.Context, p autotrader.SearchParams) (int, error) {
	n, err := h.count(ctx, p)
	if err != nil {
		return 0, err
	}
	if n > autotrader.MaxReachable {
		return -1, nil
	}
	return n, nil
}

func (h *harvester) count(ctx context.Context, p autotrader.SearchParams) (int, error) {
	p.NumRecords = 1
	p.FirstRecord = 0
	res, err := h.client.Search(ctx, p)
	h.queries++
	time.Sleep(h.pause)
	if err != nil {
		return 0, err
	}
	return res.TotalResultCount, nil
}

// drain pulls every reachable record from a shard already known to fit.
func (h *harvester) drain(ctx context.Context, p autotrader.SearchParams, label string) error {
	before := len(h.seen)
	for first := 0; first <= autotrader.MaxFirstRecord; first += autotrader.MaxNumRecords {
		p.FirstRecord = first
		p.NumRecords = autotrader.MaxNumRecords

		res, err := h.client.Search(ctx, p)
		h.queries++
		time.Sleep(h.pause)
		if err != nil {
			return err
		}
		for _, l := range res.Listings {
			h.seen[l.ID] = l
		}
		if len(res.Listings) < autotrader.MaxNumRecords {
			break
		}
	}
	fmt.Fprintf(os.Stderr, "  %-44s +%-4d running=%d\n", label, len(h.seen)-before, len(h.seen))
	return nil
}
