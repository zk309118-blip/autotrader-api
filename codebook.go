package autotrader

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

// snapshot is a vendored copy of the reference endpoint so the codebook is
// readable offline and the valid filter values are self-documenting. Refresh
// it with `atc codebook --refresh > data/searchoptions.json`.
//
//go:embed data/searchoptions.json
var snapshot []byte

// Codebook is the full vocabulary of valid filter values: every make with its
// model and trim tree, plus the enumerations for colors, body styles,
// features, mileage bands and the rest.
type Codebook struct {
	ListingTypes []CodeName `json:"listingTypes"`
	SellerTypes  []CodeName `json:"sellerType"`
	BodyStyles   []CodeName `json:"bodystyles"`
	DealTypes    []CodeName `json:"dealTypes"`
	DoorCodes    []CodeName `json:"doorCode"`
	DriveGroups  []CodeName `json:"driveGroup"`
	EngineCodes  []CodeName `json:"engineCode"`
	ExtColors    []CodeName `json:"extColorGroups"`
	IntColors    []CodeName `json:"intColorGroups"`
	FuelTypes    []CodeName `json:"fuelTypeGroup"`
	Features     []Feature  `json:"featureCode"`
	Makes        []Make     `json:"makeCode"`
	MaximumSeats []CodeName `json:"maximumSeats"`
	MileageBands []CodeName `json:"mileageRanges"`
	PriceBands   []CodeName `json:"priceRanges"`
	Radiuses     []CodeName `json:"searchRadiuses"`
	Transmission []CodeName `json:"transmissionGroups"`
	StyleCodes   []CodeName `json:"vehicleStyleCode"`
	History      []CodeName `json:"vehicleHistoryOptions"`
	Years        []int      `json:"years"`
}

// Make is a manufacturer and its model tree.
type Make struct {
	Code   string  `json:"code"`
	Name   string  `json:"name"`
	Models []Model `json:"models"`
}

// Model is a nameplate and its trims.
type Model struct {
	Code  string     `json:"code"`
	Name  string     `json:"name"`
	Trims []CodeName `json:"trims"`
}

// Feature is an equipment filter, grouped for display.
type Feature struct {
	Code         string `json:"code"`
	Name         string `json:"name"`
	DisplayGroup string `json:"displayGroup"`
}

type codebookEnvelope struct {
	Success bool     `json:"success"`
	Payload Codebook `json:"payload"`
}

// Codebook fetches the current reference data from the API.
func (c *Client) Codebook(ctx context.Context) (*Codebook, error) {
	body, err := c.get(ctx, codebookPath, nil)
	if err != nil {
		return nil, err
	}
	var env codebookEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("autotrader: decoding codebook: %w", err)
	}
	return &env.Payload, nil
}

// CodebookRaw returns the undecoded reference JSON.
func (c *Client) CodebookRaw(ctx context.Context) ([]byte, error) {
	return c.get(ctx, codebookPath, nil)
}

// EmbeddedCodebook returns the vendored snapshot without any network call.
// It may lag the live data as models are added each model year.
func EmbeddedCodebook() (*Codebook, error) {
	var env codebookEnvelope
	if err := json.Unmarshal(snapshot, &env); err != nil {
		return nil, fmt.Errorf("autotrader: decoding embedded codebook: %w", err)
	}
	return &env.Payload, nil
}

// FindMake resolves a make by code or display name, case-insensitively.
func (c *Codebook) FindMake(s string) (Make, bool) {
	for _, m := range c.Makes {
		if strings.EqualFold(m.Code, s) || strings.EqualFold(m.Name, s) {
			return m, true
		}
	}
	return Make{}, false
}

// FindModel resolves a model within a make by code or display name.
func (m Make) FindModel(s string) (Model, bool) {
	for _, mo := range m.Models {
		if strings.EqualFold(mo.Code, s) || strings.EqualFold(mo.Name, s) {
			return mo, true
		}
	}
	return Model{}, false
}
