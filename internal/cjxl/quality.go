// Package cjxl builds cjxl command lines, runs the encoder, and converts between
// the distance and quality expressions of the same setting.
package cjxl

import "math"

// Distance and quality are two displays of one stored quantity. jxleet stores
// distance; quality is a display transform only (see README "Distance, quality
// and effort"). The formulas mirror libjxl's JxlEncoderDistanceFromQuality so
// the toggle agrees with what cjxl -q would do internally.

// DistanceFromQuality converts a quality value (0..100) to a distance value.
// 100 = mathematically lossless (distance 0).
func DistanceFromQuality(quality float64) float64 {
	switch {
	case quality >= 100:
		return 0
	case quality >= 30:
		return 0.1 + (100-quality)*0.09
	default:
		return 53.0/3000.0*quality*quality - 23.0/20.0*quality + 25.0
	}
}

// QualityFromDistance is the inverse of DistanceFromQuality, clamped to 0..100.
// It is best-effort for distances below 0.1, where the forward mapping is
// discontinuous (any quality just under 100 maps to ~0.1, while 100 maps to 0).
func QualityFromDistance(distance float64) float64 {
	switch {
	case distance <= 0:
		return 100
	case distance <= 6.4: // linear region: quality in [30, 100)
		return clampQuality(100 - (distance-0.1)/0.09)
	default: // quadratic region: quality in [0, 30)
		const a = 53.0 / 3000.0
		const b = -23.0 / 20.0
		c := 25.0 - distance
		disc := b*b - 4*a*c
		if disc < 0 {
			return 0
		}
		// Smaller root falls in the [0, 30) branch.
		q := (-b - math.Sqrt(disc)) / (2 * a)
		return clampQuality(q)
	}
}

func clampQuality(q float64) float64 {
	if q < 0 {
		return 0
	}
	if q > 100 {
		return 100
	}
	return q
}
