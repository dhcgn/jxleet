package output

import (
	"strings"
	"testing"

	"github.com/dhcgn/jxleet/internal/cjxl"
	"github.com/dhcgn/jxleet/internal/routes"
)

func TestSettingsSuffix(t *testing.T) {
	cases := []struct {
		distance float64
		effort   int
		version  string
		want     string
	}{
		{1, 7, "0.11.1", "d1.00-e7-cjxl0.11.1"},
		{0, 7, "0.11.1", "d0.00-e7-cjxl0.11.1"},
		{0.3, 9, "0.11.1", "d0.30-e9-cjxl0.11.1"},
		{1.5, 7, "", "d1.50-e7"},
	}
	for _, c := range cases {
		if got := SettingsSuffix(c.distance, c.effort, c.version); got != c.want {
			t.Errorf("SettingsSuffix(%v, %d, %q) = %q, want %q", c.distance, c.effort, c.version, got, c.want)
		}
	}
}

func TestSuffixForTranscodeIgnoresDistance(t *testing.T) {
	args := []cjxl.Arg{{Key: "--lossless_jpeg", Value: "1"}}
	if got := SuffixFor(routes.RouteTranscode, args, "0.11.1"); got != "d0.00-e7-cjxl0.11.1" {
		t.Errorf("transcode suffix = %q", got)
	}
}

func TestSuffixForEncode(t *testing.T) {
	args := []cjxl.Arg{{Key: "-d", Value: "0.3"}, {Key: "-e", Value: "9"}}
	if got := SuffixFor(routes.RouteEncode, args, "0.11.1"); got != "d0.30-e9-cjxl0.11.1" {
		t.Errorf("encode suffix = %q", got)
	}
}

func TestSuffixForQualityRule(t *testing.T) {
	args := []cjxl.Arg{{Key: "-q", Value: "90"}, {Key: "-e", Value: "8"}}
	got := SuffixFor(routes.RouteEncode, args, "0.11.1")
	if !strings.HasSuffix(got, "-e8-cjxl0.11.1") || !strings.HasPrefix(got, "d") {
		t.Errorf("quality suffix = %q", got)
	}
}
