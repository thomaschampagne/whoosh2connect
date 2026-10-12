// Package device defines spoofable Garmin device presets and applies them to
// FIT activity messages. Product IDs come verbatim from the Garmin FIT SDK
// product list (muktihari/fit profile/typedef) — nothing is guessed.
package device

import (
	"fmt"
	"sort"

	"github.com/muktihari/fit/profile/typedef"
)

// Manufacturer is Garmin (1).
const Manufacturer = typedef.ManufacturerGarmin

// Preset is a named, known Garmin device.
type Preset struct {
	ID          string
	Product     typedef.GarminProduct
	DefaultName string
}

// presets maps stable IDs to Garmin FIT SDK products.
var presets = []Preset{
	{ID: "edge_20", Product: typedef.GarminProductEdge20, DefaultName: "Edge 20"},
	{ID: "edge_25", Product: typedef.GarminProductEdge25, DefaultName: "Edge 25"},
	{ID: "edge_130", Product: typedef.GarminProductEdge130, DefaultName: "Edge 130"},
	{ID: "edge_130_plus", Product: typedef.GarminProductEdge130Plus, DefaultName: "Edge 130 Plus"},
	{ID: "edge_500", Product: typedef.GarminProductEdge500, DefaultName: "Edge 500"},
	{ID: "edge_510", Product: typedef.GarminProductEdge510, DefaultName: "Edge 510"},
	{ID: "edge_520", Product: typedef.GarminProductEdge520, DefaultName: "Edge 520"},
	{ID: "edge_520_plus", Product: typedef.GarminProductEdge520Plus, DefaultName: "Edge 520 Plus"},
	{ID: "edge_530", Product: typedef.GarminProductEdge530, DefaultName: "Edge 530"},
	{ID: "edge_540", Product: typedef.GarminProductEdge540, DefaultName: "Edge 540"},
	{ID: "edge_550", Product: typedef.GarminProductEdge550, DefaultName: "Edge 550"},
	{ID: "edge_820", Product: typedef.GarminProductEdge820, DefaultName: "Edge 820"},
	{ID: "edge_830", Product: typedef.GarminProductEdge830, DefaultName: "Edge 830"},
	{ID: "edge_840", Product: typedef.GarminProductEdge840, DefaultName: "Edge 840"},
	{ID: "edge_850", Product: typedef.GarminProductEdge850, DefaultName: "Edge 850"},
	{ID: "edge_1000", Product: typedef.GarminProductEdge1000, DefaultName: "Edge 1000"},
	{ID: "edge_1030", Product: typedef.GarminProductEdge1030, DefaultName: "Edge 1030"},
	{ID: "edge_1030_plus", Product: typedef.GarminProductEdge1030Plus, DefaultName: "Edge 1030 Plus"},
	{ID: "edge_1040", Product: typedef.GarminProductEdge1040, DefaultName: "Edge 1040"},
	{ID: "edge_1050", Product: typedef.GarminProductEdge1050, DefaultName: "Edge 1050"},
	{ID: "edge_explore", Product: typedef.GarminProductEdgeExplore, DefaultName: "Edge Explore"},
	{ID: "edge_explore_2", Product: typedef.GarminProductEdgeExplore2, DefaultName: "Edge Explore 2"},
}

// Presets returns all device presets sorted by display name.
func Presets() []Preset {
	out := make([]Preset, len(presets))
	copy(out, presets)
	sort.Slice(out, func(i, j int) bool { return out[i].DefaultName < out[j].DefaultName })
	return out
}

// Lookup finds a preset by ID. Unknown IDs get a helpful error.
func Lookup(id string) (Preset, error) {
	for _, p := range presets {
		if p.ID == id {
			return p, nil
		}
	}
	return Preset{}, fmt.Errorf("unknown device preset %q", id)
}
