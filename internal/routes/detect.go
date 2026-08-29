package routes

import (
	"bytes"
	"strings"
)

// DetectFormat identifies the input format from content, using the filename
// extension only as a hint/tiebreaker. Content-based detection is preferred so a
// mislabelled file still routes correctly. Returns FormatUnknown when the file
// is not a format cjxl accepts.
func DetectFormat(header []byte, filename string) Format {
	if f := detectByMagic(header); f != FormatUnknown {
		return f
	}
	return detectByExt(filename)
}

func detectByMagic(b []byte) Format {
	switch {
	case len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF:
		return FormatJPEG
	case len(b) >= 8 && bytes.Equal(b[:8], []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}):
		// APNG is a PNG with an acTL chunk; distinguished below if present.
		if bytes.Contains(b, []byte("acTL")) {
			return FormatAPNG
		}
		return FormatPNG
	case len(b) >= 6 && (bytes.Equal(b[:6], []byte("GIF87a")) || bytes.Equal(b[:6], []byte("GIF89a"))):
		return FormatGIF
	case len(b) >= 4 && bytes.Equal(b[:4], []byte{0x76, 0x2F, 0x31, 0x01}):
		return FormatEXR
	case len(b) >= 2 && b[0] == 'P' && b[1] == '7':
		return FormatPAM
	case len(b) >= 2 && b[0] == 'P' && (b[1] == '2' || b[1] == '5'):
		return FormatPGM
	case len(b) >= 2 && b[0] == 'P' && (b[1] == '3' || b[1] == '6'):
		return FormatPPM
	case len(b) >= 2 && b[0] == 'P' && (b[1] == 'f' || b[1] == 'F'):
		return FormatPFM
	case len(b) >= 12 && bytes.Equal(b[4:12], []byte{'J', 'X', 'L', ' ', 0x0D, 0x0A, 0x87, 0x0A}):
		return FormatJXL
	case len(b) >= 2 && b[0] == 0xFF && b[1] == 0x0A:
		// Raw JXL codestream.
		return FormatJXL
	case len(b) >= 3 && b[0] == 'P' && b[1] == 'G' && b[2] == ' ':
		// PGX starts with the ASCII "PG " marker.
		return FormatPGX
	}
	return FormatUnknown
}

var extFormats = map[string]Format{
	".jpg":  FormatJPEG,
	".jpeg": FormatJPEG,
	".jpe":  FormatJPEG,
	".png":  FormatPNG,
	".apng": FormatAPNG,
	".gif":  FormatGIF,
	".exr":  FormatEXR,
	".ppm":  FormatPPM,
	".pgm":  FormatPGM,
	".pam":  FormatPAM,
	".pfm":  FormatPFM,
	".pgx":  FormatPGX,
	".jxl":  FormatJXL,
}

func detectByExt(filename string) Format {
	dot := strings.LastIndex(filename, ".")
	if dot < 0 {
		return FormatUnknown
	}
	if f, ok := extFormats[strings.ToLower(filename[dot:])]; ok {
		return f
	}
	return FormatUnknown
}
