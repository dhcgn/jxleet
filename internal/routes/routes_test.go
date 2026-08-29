package routes

import "testing"

func TestDetermine(t *testing.T) {
	tests := []struct {
		name         string
		format       Format
		losslessJPEG bool
		want         Route
	}{
		{"jpeg lossless -> transcode", FormatJPEG, true, RouteTranscode},
		{"jpeg lossy -> reencode", FormatJPEG, false, RouteReencode},
		{"jxl always reencode (lossless flag ignored)", FormatJXL, true, RouteReencode},
		{"jxl reencode", FormatJXL, false, RouteReencode},
		{"png -> encode", FormatPNG, false, RouteEncode},
		{"gif -> encode", FormatGIF, true, RouteEncode},
		{"exr -> encode", FormatEXR, false, RouteEncode},
		{"pfm -> encode", FormatPFM, false, RouteEncode},
		{"unknown -> skip", FormatUnknown, true, RouteSkip},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Determine(tt.format, tt.losslessJPEG); got != tt.want {
				t.Errorf("Determine(%q, %v) = %v, want %v", tt.format, tt.losslessJPEG, got, tt.want)
			}
		})
	}
}

func TestReversible(t *testing.T) {
	if !RouteTranscode.Reversible(1.0) {
		t.Error("transcode must always be reversible")
	}
	if !RouteEncode.Reversible(0) {
		t.Error("encode at distance 0 must be reversible")
	}
	if RouteEncode.Reversible(1.0) {
		t.Error("encode at distance 1.0 must not be reversible")
	}
	if RouteReencode.Reversible(0) {
		t.Error("reencode is never reversible")
	}
}

func TestDominant(t *testing.T) {
	tests := []struct {
		name   string
		counts map[Route]int
		want   Route
	}{
		{"most files wins", map[Route]int{RouteTranscode: 12, RouteEncode: 30}, RouteEncode},
		{"tie breaks to more consequential", map[Route]int{RouteReencode: 5, RouteEncode: 5}, RouteReencode},
		{"transcode only", map[Route]int{RouteTranscode: 3}, RouteTranscode},
		{"empty -> skip", map[Route]int{}, RouteSkip},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Dominant(tt.counts); got != tt.want {
				t.Errorf("Dominant(%v) = %v, want %v", tt.counts, got, tt.want)
			}
		})
	}
}

func TestDetectFormat(t *testing.T) {
	tests := []struct {
		name     string
		header   []byte
		filename string
		want     Format
	}{
		{"jpeg magic", []byte{0xFF, 0xD8, 0xFF, 0xE0}, "x.bin", FormatJPEG},
		{"png magic", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, "x", FormatPNG},
		{"gif magic", []byte("GIF89a....."), "x", FormatGIF},
		{"ext fallback", nil, "photo.JPG", FormatJPEG},
		{"jxl by ext", nil, "a.jxl", FormatJXL},
		{"raw jxl codestream", []byte{0xFF, 0x0A}, "x", FormatJXL},
		{"unknown", []byte("random"), "notes.txt", FormatUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DetectFormat(tt.header, tt.filename); got != tt.want {
				t.Errorf("DetectFormat(%v, %q) = %q, want %q", tt.header, tt.filename, got, tt.want)
			}
		})
	}
}
