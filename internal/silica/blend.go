package silica

// blendKeys maps Procreate's blend index onto the four-character Photoshop
// blend-mode key written into a PSD layer record.
//
// Index 0 (Normal) covers the overwhelming majority of real layers. The rest of
// the table follows Procreate's published blend ordering; it is applied as-is
// rather than verified against Photoshop's rendering, which is why BlendKey
// reports whether an index was known at all and the exporter surfaces the ones
// that were not.
var blendKeys = map[int]string{
	0:  "norm", // Normal
	1:  "mul ", // Multiply
	2:  "scrn", // Screen
	3:  "lddg", // Add / Linear Dodge
	4:  "lite", // Lighten
	5:  "smud", // Exclusion
	6:  "diff", // Difference
	7:  "fsub", // Subtract
	8:  "lbrn", // Linear Burn
	9:  "div ", // Color Dodge
	10: "idiv", // Color Burn
	11: "over", // Overlay
	12: "hLit", // Hard Light
	13: "vLit", // Vivid Light
	14: "lLit", // Linear Light
	15: "pLit", // Pin Light
	16: "lgCl", // Lighter Color
	17: "dkCl", // Darker Color
	18: "sLit", // Soft Light
	19: "hue ", // Hue
	20: "sat ", // Saturation
	21: "colr", // Color
	22: "lum ", // Luminosity
	23: "dark", // Darken
}

// blendKey returns the Photoshop key for a Procreate blend index, and whether
// the index is in the table.
func blendKey(i int) (string, bool) {
	k, ok := blendKeys[i]
	if !ok {
		return "norm", false
	}
	return k, true
}

// BlendKey is blendKey for callers outside the package.
func BlendKey(i int) (string, bool) { return blendKey(i) }
