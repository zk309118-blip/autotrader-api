package autotrader

import (
	"net/url"
	"strconv"
)

// Sort values accepted by SearchParams.SortBy.
const (
	SortRelevance   = "relevance"
	SortPriceAsc    = "derivedpriceASC"
	SortPriceDesc   = "derivedpriceDESC"
	SortMileageAsc  = "mileageASC"
	SortYearDesc    = "yearDESC"
	SortDistanceAsc = "distanceASC"
	SortNewestFirst = "datelistedDESC"
)

// Listing conditions.
const (
	ConditionNew       = "NEW"
	ConditionUsed      = "USED"
	ConditionCertified = "CERTIFIED"
	Condition3PCert    = "3P_CERT"
)

// Hard limits enforced by the API. Exceeding them fails silently rather than
// erroring: NumRecords clamps to 100, and FirstRecord above 300 returns an
// empty listing array with no TotalResultCount.
const (
	MaxNumRecords  = 100
	MaxFirstRecord = 300
	// MaxReachable is the total number of records any single query can yield,
	// regardless of TotalResultCount. See docs/pagination.md.
	MaxReachable = MaxFirstRecord + MaxNumRecords
)

// SearchParams builds a query against /rest/lsc/listing.
//
// Unknown or misspelled parameters are ignored by the server rather than
// rejected, so a typo silently returns unfiltered results. After adding a
// filter, confirm TotalResultCount actually moved.
type SearchParams struct {
	// Location. SearchRadius 0 means nationwide.
	Zip          string
	SearchRadius int

	// Vehicle. Slices repeat the parameter; the server ORs them.
	MakeCode  []string
	ModelCode []string
	TrimCode  []string

	ListingType []string // NEW, USED, CERTIFIED, 3P_CERT
	SellerType  string   // "d" dealer, "p" private party

	MinPrice  int
	MaxPrice  int
	StartYear int
	EndYear   int

	// Mileage takes a codebook range code such as "-60000" (60k or less) or
	// "100001-". It is NOT called maxMileage; that parameter does nothing.
	Mileage string

	DriveGroup         []string
	FuelTypeGroup      []string
	TransmissionCode   []string
	VehicleStyleCode   []string
	ExtColorSimple     []string
	IntColorSimple     []string
	VehicleHistoryType []string
	FeatureCode        []string
	DealType           []string
	MaximumSeats       string
	DoorCode           string
	EngineCode         []string

	// Direct lookups. Either bypasses all other filters.
	ListingID string
	VIN       string

	// MarketExtension "include" widens beyond the radius to nearby markets.
	MarketExtension string

	SortBy      string
	NumRecords  int
	FirstRecord int
}

// Values renders the params as a query string, applying defaults and clamping
// NumRecords/FirstRecord to the server's real limits.
func (p SearchParams) Values() url.Values {
	v := url.Values{}

	set := func(k, val string) {
		if val != "" {
			v.Set(k, val)
		}
	}
	setInt := func(k string, val int) {
		if val != 0 {
			v.Set(k, strconv.Itoa(val))
		}
	}
	add := func(k string, vals []string) {
		for _, s := range vals {
			if s != "" {
				v.Add(k, s)
			}
		}
	}

	set("zip", p.Zip)
	// SearchRadius 0 is meaningful (national), so it is always sent.
	v.Set("searchRadius", strconv.Itoa(p.SearchRadius))

	add("makeCode", p.MakeCode)
	add("modelCode", p.ModelCode)
	add("trimCode", p.TrimCode)
	add("listingType", p.ListingType)
	add("driveGroup", p.DriveGroup)
	add("fuelTypeGroup", p.FuelTypeGroup)
	add("transmissionCode", p.TransmissionCode)
	add("vehicleStyleCode", p.VehicleStyleCode)
	add("extColorSimple", p.ExtColorSimple)
	add("intColorSimple", p.IntColorSimple)
	add("vehicleHistoryType", p.VehicleHistoryType)
	add("featureCode", p.FeatureCode)
	add("dealType", p.DealType)
	add("engineCode", p.EngineCode)

	set("sellerType", p.SellerType)
	set("mileage", p.Mileage)
	set("maximumSeats", p.MaximumSeats)
	set("doorCode", p.DoorCode)
	set("marketExtension", p.MarketExtension)
	set("listingId", p.ListingID)
	set("vin", p.VIN)

	setInt("minPrice", p.MinPrice)
	setInt("maxPrice", p.MaxPrice)
	setInt("startYear", p.StartYear)
	setInt("endYear", p.EndYear)

	sortBy := p.SortBy
	if sortBy == "" {
		sortBy = SortRelevance
	}
	v.Set("sortBy", sortBy)

	n := p.NumRecords
	if n <= 0 {
		n = 25
	}
	if n > MaxNumRecords {
		n = MaxNumRecords
	}
	v.Set("numRecords", strconv.Itoa(n))

	first := p.FirstRecord
	if first < 0 {
		first = 0
	}
	v.Set("firstRecord", strconv.Itoa(first))

	v.Set("channel", "ATC")
	return v
}
