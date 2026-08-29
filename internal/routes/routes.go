// Package routes models jxleet's central concept: every input file takes one of
// three routes, and the route follows from the input format times the active
// preset — it is not a fixed property of the file (see README "The three
// routes").
package routes

// Format is an input image format that cjxl accepts. jxleet adds none and
// removes none beyond what cjxl supports.
type Format string

// Input formats cjxl accepts, as used in preset match rules.
const (
	FormatJPEG    Format = "JPEG"
	FormatPNG     Format = "PNG"
	FormatAPNG    Format = "APNG"
	FormatGIF     Format = "GIF"
	FormatEXR     Format = "EXR"
	FormatPPM     Format = "PPM"
	FormatPGM     Format = "PGM"
	FormatPAM     Format = "PAM"
	FormatPFM     Format = "PFM"
	FormatPGX     Format = "PGX"
	FormatJXL     Format = "JXL"
	FormatUnknown Format = ""
)

// Route is one of the three operations jxleet routes a file through.
type Route int

const (
	// RouteSkip means the file is unsupported and is reported, not converted.
	RouteSkip Route = iota
	// RouteTranscode repacks a JPEG losslessly (--lossless_jpeg=1). Reversible:
	// djxl restores the original JPEG byte for byte.
	RouteTranscode
	// RouteReencode decodes and re-encodes (JPEG with --lossless_jpeg=0, or any
	// JXL input). Not reversible.
	RouteReencode
	// RouteEncode encodes from pixels (PNG, GIF, EXR, NetPBM, PFM, PGX).
	// Lossless only at distance 0.
	RouteEncode
)

// String returns the human-readable route name used in badges and logs.
func (r Route) String() string {
	switch r {
	case RouteTranscode:
		return "Transcode"
	case RouteReencode:
		return "Reencode"
	case RouteEncode:
		return "Encode"
	default:
		return "Skip"
	}
}

// Reversible reports whether the route can be reversed to the original bytes.
// Transcode is always reversible; Encode is reversible only at distance 0.
func (r Route) Reversible(distance float64) bool {
	switch r {
	case RouteTranscode:
		return true
	case RouteEncode:
		return distance == 0
	default:
		return false
	}
}

// DefaultColors are the built-in route colours used across the UI. They can be
// overridden via config.
var DefaultColors = map[Route]string{
	RouteTranscode: "#22c55e", // green
	RouteReencode:  "#f97316", // orange
	RouteEncode:    "#3b82f6", // blue
	RouteSkip:      "#6b7280", // grey
}

// Determine returns the route for a file of the given format under the effective
// --lossless_jpeg setting for that file.
//
// losslessJPEG only affects JPEG inputs: 1 -> Transcode, 0 -> Reencode. A JXL
// input is always Reencode. All other supported formats are Encode. Unknown
// formats are Skip.
func Determine(f Format, losslessJPEG bool) Route {
	switch f {
	case FormatJPEG:
		if losslessJPEG {
			return RouteTranscode
		}
		return RouteReencode
	case FormatJXL:
		return RouteReencode
	case FormatPNG, FormatAPNG, FormatGIF, FormatEXR,
		FormatPPM, FormatPGM, FormatPAM, FormatPFM, FormatPGX:
		return RouteEncode
	default:
		return RouteSkip
	}
}

// Dominant returns the route that should colour a batch's Convert button: the
// route with the most files, breaking ties by irreversibility (Reencode over
// Encode over Transcode) so the more consequential route wins.
func Dominant(counts map[Route]int) Route {
	order := []Route{RouteReencode, RouteEncode, RouteTranscode}
	best := RouteSkip
	bestN := 0
	for _, r := range order {
		if counts[r] > bestN {
			best, bestN = r, counts[r]
		}
	}
	return best
}
