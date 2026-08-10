// Command atc queries autotrader.com's internal listing API.
//
//	atc search --make TOYOTA --model TACOMA --zip 63101 --max-price 35000
//	atc vin 5TFSX5EN7GX044911
//	atc codebook --makes
//	atc codebook --models TOYOTA
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	autotrader "github.com/dhruvkar/autotrader-api"
)

const usage = `atc - query the autotrader.com listing API

usage:
  atc search  [flags]     search listings
  atc vin     <VIN>       look up one vehicle by VIN
  atc listing <ID>        look up one vehicle by listing id
  atc codebook [flags]    print valid filter values

run "atc <command> -h" for flags.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "search":
		err = cmdSearch(os.Args[2:])
	case "vin":
		err = cmdLookup(os.Args[2:], "vin")
	case "listing":
		err = cmdLookup(os.Args[2:], "listing")
	case "codebook":
		err = cmdCodebook(os.Args[2:])
	case "-h", "--help", "help":
		fmt.Print(usage)
		return
	default:
		fmt.Fprintf(os.Stderr, "atc: unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}

	if err != nil {
		if errors.Is(err, autotrader.ErrBlocked) {
			fmt.Fprintln(os.Stderr, "atc: blocked by bot protection.")
			fmt.Fprintln(os.Stderr, "     This API rejects datacenter IPs. If you are on a cloud VM,")
			fmt.Fprintln(os.Stderr, "     run from a residential connection instead.")
			fmt.Fprintln(os.Stderr, "     See docs/bot-protection.md.")
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "atc: %v\n", err)
		os.Exit(1)
	}
}

// multiFlag collects a repeatable string flag.
type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }
func (m *multiFlag) Set(s string) error {
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			*m = append(*m, part)
		}
	}
	return nil
}

func cmdSearch(args []string) error {
	fs := flag.NewFlagSet("search", flag.ExitOnError)

	var makes, models, condition, drive, fuel, body, history multiFlag
	fs.Var(&makes, "make", "make code, repeatable (e.g. TOYOTA)")
	fs.Var(&models, "model", "model code, repeatable (e.g. TACOMA)")
	fs.Var(&condition, "condition", "NEW, USED, CERTIFIED, 3P_CERT")
	fs.Var(&drive, "drive", "AWD4WD, FWD, RWD")
	fs.Var(&fuel, "fuel", "GSL, DSL, HYB, ELE")
	fs.Var(&body, "body", "body style code (e.g. SUVCROSS, SEDAN)")
	fs.Var(&history, "history", "ONE_OWNER, CLEAN_TITLE, NO_ACCIDENTS, NO_FRAME_DAMAGE")

	zip := fs.String("zip", "63101", "origin ZIP code")
	radius := fs.Int("radius", 50, "search radius in miles; 0 means nationwide")
	minPrice := fs.Int("min-price", 0, "minimum price")
	maxPrice := fs.Int("max-price", 0, "maximum price")
	startYear := fs.Int("start-year", 0, "earliest model year")
	endYear := fs.Int("end-year", 0, "latest model year")
	mileage := fs.String("mileage", "", `mileage band code, e.g. "-60000" (not a raw number)`)
	seller := fs.String("seller", "", `"d" dealer or "p" private party`)
	sortBy := fs.String("sort", autotrader.SortRelevance, "relevance|derivedpriceASC|derivedpriceDESC|mileageASC|yearDESC|distanceASC|datelistedDESC")
	limit := fs.Int("limit", 25, "records to return (max 400; the API caps a single query there)")
	asJSON := fs.Bool("json", false, "emit raw JSON listings instead of a table")

	if err := fs.Parse(args); err != nil {
		return err
	}

	p := autotrader.SearchParams{
		Zip:                *zip,
		SearchRadius:       *radius,
		MakeCode:           makes,
		ModelCode:          models,
		ListingType:        condition,
		DriveGroup:         drive,
		FuelTypeGroup:      fuel,
		VehicleStyleCode:   body,
		VehicleHistoryType: history,
		SellerType:         *seller,
		MinPrice:           *minPrice,
		MaxPrice:           *maxPrice,
		StartYear:          *startYear,
		EndYear:            *endYear,
		Mileage:            *mileage,
		SortBy:             *sortBy,
	}

	c := autotrader.New()
	ctx := context.Background()

	want := *limit
	if want > autotrader.MaxReachable {
		fmt.Fprintf(os.Stderr, "atc: --limit capped at %d (API ceiling for one query)\n", autotrader.MaxReachable)
		want = autotrader.MaxReachable
	}

	var all []autotrader.Listing
	var total int
	owners := map[int64]autotrader.Owner{}

	for first := 0; first < want; first += autotrader.MaxNumRecords {
		p.FirstRecord = first
		p.NumRecords = min(autotrader.MaxNumRecords, want-first)

		res, err := c.Search(ctx, p)
		if err != nil {
			return err
		}
		total = res.TotalResultCount
		for id, o := range res.OwnerByID() {
			owners[id] = o
		}
		all = append(all, res.Listings...)
		if len(res.Listings) < p.NumRecords {
			break
		}
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(all)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "YEAR\tVEHICLE\tMILES\tPRICE\tCOND\tDEALER\tVIN")
	for _, l := range all {
		price := "-"
		if v, ok := l.Pricing.Price(); ok {
			price = fmt.Sprintf("$%d", v)
		} else if l.Pricing.NoPriceLabel != "" {
			price = "(call)"
		}
		miles := "-"
		if m, ok := l.Miles(); ok {
			miles = fmt.Sprintf("%d", m)
		}
		name := strings.TrimSpace(l.Make.Name + " " + l.Model.Name)
		dealer := owners[l.OwnerID].Name
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\t%s\n",
			l.Year, truncate(name, 30), miles, price, l.ListingType, truncate(dealer, 26), l.VIN)
	}
	w.Flush()

	fmt.Printf("\n%d shown | %d total matches", len(all), total)
	if total > autotrader.MaxReachable {
		fmt.Printf(" | only %d reachable per query -- partition to get the rest (see examples/sharding)", autotrader.MaxReachable)
	}
	fmt.Println()
	return nil
}

func cmdLookup(args []string, kind string) error {
	fs := flag.NewFlagSet(kind, flag.ExitOnError)
	asJSON := fs.Bool("json", false, "emit raw JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: atc %s <value>", kind)
	}

	c := autotrader.New()
	ctx := context.Background()

	var (
		l   *autotrader.Listing
		err error
	)
	if kind == "vin" {
		l, err = c.ByVIN(ctx, fs.Arg(0))
	} else {
		l, err = c.ByListingID(ctx, fs.Arg(0))
	}
	if err != nil {
		return err
	}
	if l == nil {
		return fmt.Errorf("no listing found for %q", fs.Arg(0))
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(l)
	}

	price := "-"
	if v, ok := l.Pricing.Price(); ok {
		price = fmt.Sprintf("$%d", v)
	}
	miles := "-"
	if m, ok := l.Miles(); ok {
		miles = fmt.Sprintf("%d", m)
	}
	fmt.Printf("%s\n", l.TitleLong)
	fmt.Printf("  vin        %s\n", l.VIN)
	fmt.Printf("  listing    %d\n", l.ID)
	fmt.Printf("  price      %s\n", price)
	fmt.Printf("  mileage    %s\n", miles)
	fmt.Printf("  condition  %s\n", l.ListingType)
	fmt.Printf("  drivetrain %s / %s / %s\n", l.DriveType.Name, l.Engine.Name, l.Transmission.Name)
	fmt.Printf("  color      %s on %s\n", l.Color.ExteriorColor, l.Color.InteriorColor)
	fmt.Printf("  on site    %d days\n", l.DaysOnSite)
	fmt.Printf("  url        %s\n", l.URL())
	return nil
}

func cmdCodebook(args []string) error {
	fs := flag.NewFlagSet("codebook", flag.ExitOnError)
	listMakes := fs.Bool("makes", false, "list every make code")
	forMake := fs.String("models", "", "list model codes for a make")
	refresh := fs.Bool("refresh", false, "fetch live reference JSON and print it to stdout")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *refresh {
		raw, err := autotrader.New().CodebookRaw(context.Background())
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(raw)
		return err
	}

	cb, err := autotrader.EmbeddedCodebook()
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer w.Flush()

	switch {
	case *listMakes:
		fmt.Fprintln(w, "CODE\tNAME\tMODELS")
		for _, m := range cb.Makes {
			fmt.Fprintf(w, "%s\t%s\t%d\n", m.Code, m.Name, len(m.Models))
		}
	case *forMake != "":
		m, ok := cb.FindMake(*forMake)
		if !ok {
			return fmt.Errorf("unknown make %q (try: atc codebook --makes)", *forMake)
		}
		fmt.Fprintln(w, "CODE\tNAME\tTRIMS")
		for _, mo := range m.Models {
			fmt.Fprintf(w, "%s\t%s\t%d\n", mo.Code, mo.Name, len(mo.Trims))
		}
	default:
		fmt.Fprintln(w, "SECTION\tVALUES")
		fmt.Fprintf(w, "makes\t%d\n", len(cb.Makes))
		fmt.Fprintf(w, "conditions\t%s\n", codes(cb.ListingTypes))
		fmt.Fprintf(w, "drive\t%s\n", codes(cb.DriveGroups))
		fmt.Fprintf(w, "fuel\t%s\n", codes(cb.FuelTypes))
		fmt.Fprintf(w, "transmission\t%s\n", codes(cb.Transmission))
		fmt.Fprintf(w, "body styles\t%s\n", codes(cb.StyleCodes))
		fmt.Fprintf(w, "ext colors\t%s\n", codes(cb.ExtColors))
		fmt.Fprintf(w, "mileage bands\t%s\n", codes(cb.MileageBands))
		fmt.Fprintf(w, "radiuses\t%s\n", codes(cb.Radiuses))
		fmt.Fprintf(w, "history\t%s\n", codes(cb.History))
		fmt.Fprintf(w, "deal types\t%s\n", codes(cb.DealTypes))
		fmt.Fprintln(w, "\nhint: atc codebook --makes | atc codebook --models TOYOTA")
	}
	return nil
}

func codes(items []autotrader.CodeName) string {
	out := make([]string, 0, len(items))
	for _, i := range items {
		out = append(out, i.Code)
	}
	return strings.Join(out, " ")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}
