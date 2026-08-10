package autotrader

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// blockedBody is the interstitial the bot gate actually serves. Note the 200.
const blockedBody = `<!DOCTYPE html>
<html><head><title>Autotrader - page unavailable</title></head>
<body>Sorry, the page you requested is unavailable.</body></html>`

func TestBlockedInterstitialIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK) // the whole trap: a block looks like success
		_, _ = w.Write([]byte(blockedBody))
	}))
	defer srv.Close()

	c := New(WithBaseURL(srv.URL))
	_, err := c.Search(context.Background(), SearchParams{Zip: "63101"})
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("want ErrBlocked, got %v", err)
	}
}

func TestSearchDecodes(t *testing.T) {
	const body = `{
	  "totalResultCount": 92,
	  "zip": "63101",
	  "searchRadius": 100,
	  "listings": [{
	    "id": 784528754,
	    "vin": "5TFSX5EN7GX044911",
	    "year": 2016,
	    "title": "Used 2016 Toyota Tacoma SR",
	    "listingType": "USED",
	    "ownerId": 42,
	    "make": {"code": "TOYOTA", "name": "Toyota"},
	    "model": {"code": "TACOMA", "name": "Tacoma"},
	    "mileage": {"label": "Mileage", "value": "50,871"},
	    "pricingDetail": {"salePrice": 21996, "displayPrice": 22589}
	  }],
	  "owners": [{"id": 42, "name": "Watermark Ford", "phone": {"value": "6185551212"}}]
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	res, err := New(WithBaseURL(srv.URL)).Search(context.Background(), SearchParams{Zip: "63101"})
	if err != nil {
		t.Fatal(err)
	}
	if res.TotalResultCount != 92 || len(res.Listings) != 1 {
		t.Fatalf("unexpected result: %+v", res)
	}

	l := res.Listings[0]
	if got, ok := l.Miles(); !ok || got != 50871 {
		t.Errorf("Miles() = %d, %v; want 50871, true", got, ok)
	}
	if got, ok := l.Pricing.Price(); !ok || got != 21996 {
		t.Errorf("Price() = %d, %v; want 21996, true", got, ok)
	}
	if o := res.OwnerByID()[l.OwnerID]; o.Name != "Watermark Ford" {
		t.Errorf("owner join failed: %+v", o)
	}
}

// A large share of NEW listings carry a null salePrice with a "Contact Dealer
// For Price" label. Reading SalePrice directly silently drops them.
func TestPriceFallsBackWhenSalePriceIsNull(t *testing.T) {
	msrp := 58230
	display := 57990

	p := PricingDetail{NoPriceLabel: "Contact Dealer For Price", MSRP: &msrp}
	if got, ok := p.Price(); !ok || got != msrp {
		t.Errorf("msrp fallback: got %d, %v", got, ok)
	}

	p.DisplayPrice = &display
	if got, ok := p.Price(); !ok || got != display {
		t.Errorf("displayPrice should outrank msrp: got %d, %v", got, ok)
	}

	if _, ok := (PricingDetail{}).Price(); ok {
		t.Error("empty pricing should report no price")
	}
}

func TestValuesClampsToAPILimits(t *testing.T) {
	v := SearchParams{Zip: "63101", NumRecords: 5000, FirstRecord: -3}.Values()
	if v.Get("numRecords") != "100" {
		t.Errorf("numRecords not clamped: %s", v.Get("numRecords"))
	}
	if v.Get("firstRecord") != "0" {
		t.Errorf("negative firstRecord not normalised: %s", v.Get("firstRecord"))
	}
	// searchRadius=0 means national and must survive as an explicit zero.
	if v.Get("searchRadius") != "0" {
		t.Errorf("searchRadius 0 dropped: %q", v.Get("searchRadius"))
	}
}

func TestValuesRepeatsMultiValueFilters(t *testing.T) {
	v := SearchParams{MakeCode: []string{"TOYOTA", "HONDA"}}.Values()
	if got := v["makeCode"]; len(got) != 2 {
		t.Fatalf("makeCode should repeat, got %v", got)
	}
	// Empty optional fields must not be sent at all.
	if _, ok := v["sellerType"]; ok {
		t.Error("empty sellerType should be omitted")
	}
}

func TestEmbeddedCodebookLoads(t *testing.T) {
	cb, err := EmbeddedCodebook()
	if err != nil {
		t.Fatal(err)
	}
	if len(cb.Makes) < 50 {
		t.Fatalf("expected the full make list, got %d", len(cb.Makes))
	}
	m, ok := cb.FindMake("toyota")
	if !ok {
		t.Fatal("FindMake should be case-insensitive")
	}
	if _, ok := m.FindModel("Tacoma"); !ok {
		t.Error("Toyota should have a Tacoma model")
	}
}

// TestLive hits the real API. Skipped by default because it needs network and
// a residential IP; CI runs on cloud IPs, which are blocked.
func TestLive(t *testing.T) {
	if os.Getenv("ATC_LIVE") == "" {
		t.Skip("set ATC_LIVE=1 to run against the real API")
	}
	res, err := New().Search(context.Background(), SearchParams{
		Zip: "63101", SearchRadius: 50, NumRecords: 3,
	})
	if errors.Is(err, ErrBlocked) {
		t.Skip("blocked -- are you on a datacenter IP?")
	}
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Listings) == 0 {
		t.Fatal("expected listings")
	}
}
