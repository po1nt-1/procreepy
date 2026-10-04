package fixture

// approxEq compares aspect ratios with enough slack for the integer rounding a
// thumbnail's pixel dimensions introduce.
func approxEq(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 0.02
}
