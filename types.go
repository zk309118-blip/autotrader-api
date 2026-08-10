package autotrader

import (
	"strconv"
	"strings"
)

// CodeName is the API's ubiquitous {code, name} pair.
type CodeName struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// Labeled is the API's {label, value} pair. Note that Value is a display
// string, not a number -- Mileage arrives as "26,648".
type Labeled struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// SearchResult is the response from Client.Search.
//
// TotalResultCount is the true match count and is not capped -- a national
// search reports millions. It is not the number of records you can retrieve;
// see docs/pagination.md for the 400-record ceiling.
type SearchResult struct {
	Listings         []Listing `json:"listings"`
	Owners           []Owner   `json:"owners"`
	TotalResultCount int       `json:"totalResultCount"`
	SearchRadius     int       `json:"searchRadius"`
	Zip              string    `json:"zip"`
}

// Listing is one vehicle.
type Listing struct {
	ID          int64         `json:"id"`
	VIN         string        `json:"vin"`
	Year        int           `json:"year"`
	Title       string        `json:"title"`
	TitleLong   string        `json:"titleLong"`
	Make        CodeName      `json:"make"`
	Model       CodeName      `json:"model"`
	Trim        CodeName      `json:"trim"`
	ListingType string        `json:"listingType"`
	DaysOnSite  int           `json:"daysOnSite"`
	OwnerID     int64         `json:"ownerId"`
	StockID     string        `json:"stockId"`
	Mileage     Labeled       `json:"mileage"`
	Pricing     PricingDetail `json:"pricingDetail"`
	Engine      CodeName      `json:"engine"`
	BodyStyles  []CodeName    `json:"bodyStyles"`
	Images      Images        `json:"images"`

	DriveType struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"driveType"`

	FuelType struct {
		Code  string `json:"code"`
		Group string `json:"group"`
		Name  string `json:"name"`
	} `json:"fuelType"`

	Transmission struct {
		Code        string `json:"code"`
		Group       string `json:"group"`
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"transmission"`

	Color struct {
		ExteriorColor       string `json:"exteriorColor"`
		ExteriorColorSimple string `json:"exteriorColorSimple"`
		InteriorColor       string `json:"interiorColor"`
		InteriorColorSimple string `json:"interiorColorSimple"`
	} `json:"color"`
}

// Miles parses Mileage.Value ("26,648") into an integer. Returns false when
// the field is absent or unparseable, which happens on some new vehicles.
func (l Listing) Miles() (int, bool) {
	v := strings.ReplaceAll(l.Mileage.Value, ",", "")
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, false
	}
	return n, true
}

// URL returns the public detail-page URL for the listing.
func (l Listing) URL() string {
	return DefaultBaseURL + "/cars-for-sale/vehicle/" + strconv.FormatInt(l.ID, 10)
}

// PricingDetail holds the price fields. Every monetary field is a pointer
// because the API genuinely omits them: roughly 30% of NEW listings have a
// null SalePrice and carry NoPriceLabel ("Contact Dealer For Price") instead.
// Use Price() rather than reading SalePrice directly.
type PricingDetail struct {
	SalePrice             *int    `json:"salePrice"`
	DisplayPrice          *int    `json:"displayPrice"`
	MSRP                  *int    `json:"msrp"`
	Incentive             *int    `json:"incentive"`
	DealerDiscountedPrice *int    `json:"dealerDiscountedPrice"`
	PreFeeDerivedPrice    *int    `json:"preFeeDerivedPrice"`
	DealerFeesTotal       *int    `json:"dealerFeesTotal"`
	DealIndicator         string  `json:"dealIndicator"`
	NoPriceLabel          string  `json:"noPriceLabel"`
	PriceValidUntil       string  `json:"priceValidUntil"`
	KBBFairPurchase       float64 `json:"kbbFppAmount"`
	KBBDelta              float64 `json:"kbbFppDelta"`
}

// Price returns the best available price and whether one was found, falling
// back SalePrice -> DisplayPrice -> MSRP. Reading SalePrice on its own will
// drop a large share of new inventory.
func (p PricingDetail) Price() (int, bool) {
	for _, v := range []*int{p.SalePrice, p.DisplayPrice, p.MSRP} {
		if v != nil && *v > 0 {
			return *v, true
		}
	}
	return 0, false
}

// Images holds the listing photos. Sources is frequently 20+ entries.
type Images struct {
	Primary int           `json:"primary"`
	Sources []ImageSource `json:"sources"`
}

// ImageSource is one photo.
type ImageSource struct {
	Src    string `json:"src"`
	Alt    string `json:"alt"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// Owner is the selling dealer. Listings reference it by OwnerID; the full
// records arrive alongside in SearchResult.Owners.
type Owner struct {
	ID                 int64   `json:"id"`
	Name               string  `json:"name"`
	DistanceFromSearch float64 `json:"distanceFromSearch"`

	Location struct {
		Address struct {
			Address1  string  `json:"address1"`
			City      string  `json:"city"`
			State     string  `json:"state"`
			Zip       string  `json:"zip"`
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		} `json:"address"`
	} `json:"location"`

	Phone struct {
		Value   string `json:"value"`
		Visible bool   `json:"visible"`
	} `json:"phone"`

	Website struct {
		Href string `json:"href"`
	} `json:"website"`

	Rating struct {
		Value float64 `json:"value"`
		Count int     `json:"count"`
	} `json:"rating"`
}

// OwnerByID indexes the result's dealers so a Listing can be joined to its seller.
func (r *SearchResult) OwnerByID() map[int64]Owner {
	m := make(map[int64]Owner, len(r.Owners))
	for _, o := range r.Owners {
		m[o.ID] = o
	}
	return m
}
