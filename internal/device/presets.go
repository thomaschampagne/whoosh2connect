// Package device defines spoofable Garmin device presets and applies them to
// FIT activity messages. The preset list is derived from the Garmin FIT SDK
// product list shipped with muktihari/fit (profile/typedef) — nothing is
// hand-maintained or guessed.
package device

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

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

// regionSuffixes mark product variants of the same hardware sold in specific
// regions or as special editions; including them would clutter the picker
// without adding spoofable differences.
var regionSuffixes = []string{
	"_asia", "_apac", "_sea", "_china", "_japan", "_korea", "_taiwan", "_bontrager",
}

// digitBoundary turns "edge500" into "edge_500"; the library's String() forms
// are inconsistent ("edge500", "Edge_130", "edge_explore2", "edge_540").
var digitBoundary = regexp.MustCompile(`([a-z])([0-9])`)

// canonKey normalizes a library product name into a stable config key.
func canonKey(name string) string {
	return digitBoundary.ReplaceAllString(strings.ToLower(name), `${1}_${2}`)
}

// displayName prettifies a canonical key into a human label.
func displayName(key string) string {
	words := strings.Split(key, "_")
	for i, w := range words {
		if w == "" {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

// presets is built once from typedef.ListGarminProduct().
var presets = buildPresets()

// buildPresets derives the Edge device list straight from the FIT SDK product
// table, dropping regional and special-edition variants of the same hardware.
func buildPresets() []Preset {
	seen := map[typedef.GarminProduct]bool{}
	out := []Preset{}
	for _, p := range typedef.ListGarminProduct() {
		name := strings.ToLower(p.String())
		if !strings.HasPrefix(name, "edge") {
			continue
		}
		if seen[p] {
			continue
		}
		key := canonKey(name)
		if isRegional(key) {
			continue
		}
		seen[p] = true
		out = append(out, Preset{ID: key, Product: p, DefaultName: displayName(key)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DefaultName < out[j].DefaultName })
	return out
}

func isRegional(key string) bool {
	for _, s := range regionSuffixes {
		if strings.HasSuffix(key, s) {
			return true
		}
	}
	return false
}

// Presets returns all device presets sorted by display name.
func Presets() []Preset {
	out := make([]Preset, len(presets))
	copy(out, presets)
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
