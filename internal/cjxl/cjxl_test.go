package cjxl

import (
	"math"
	"reflect"
	"testing"
)

func TestQualityDistanceRoundTrip(t *testing.T) {
	// DistanceFromQuality mirrors libjxl; check known anchors.
	anchors := []struct {
		quality  float64
		distance float64
	}{
		{100, 0},
		{90, 1.0}, // 0.1 + 10*0.09
		{30, 6.4}, // 0.1 + 70*0.09
	}
	for _, a := range anchors {
		if got := DistanceFromQuality(a.quality); math.Abs(got-a.distance) > 1e-9 {
			t.Errorf("DistanceFromQuality(%v) = %v, want %v", a.quality, got, a.distance)
		}
	}

	// Inverting a distance derived from a quality returns that quality.
	for q := 5.0; q <= 99.0; q += 5 {
		d := DistanceFromQuality(q)
		back := QualityFromDistance(d)
		if math.Abs(back-q) > 1e-6 {
			t.Errorf("round trip q=%v -> d=%v -> q=%v", q, d, back)
		}
	}
}

func TestQualityFromDistanceClamps(t *testing.T) {
	if QualityFromDistance(0) != 100 {
		t.Error("distance 0 should be quality 100")
	}
	if q := QualityFromDistance(25); q < 0 || q > 1 {
		t.Errorf("distance 25 should be near quality 0, got %v", q)
	}
	if q := QualityFromDistance(1000); q != 0 {
		t.Errorf("huge distance should clamp to 0, got %v", q)
	}
}

func TestEffort(t *testing.T) {
	if EffortName(7) != "squirrel" {
		t.Errorf("effort 7 = %q, want squirrel", EffortName(7))
	}
	if EffortName(1) != "lightning" || EffortName(10) != "glacier" {
		t.Error("unexpected effort endpoint names")
	}
	if EffortName(0) != "" || EffortName(11) != "" {
		t.Error("out-of-range effort should have no name")
	}
	if !ValidEffort(7) || ValidEffort(0) || ValidEffort(11) {
		t.Error("ValidEffort range check failed")
	}
}

func TestArgsRendering(t *testing.T) {
	tests := []struct {
		name string
		args []Arg
		want []string
	}{
		{
			"short flag uses two tokens",
			[]Arg{{Key: "-d", Value: "1.0"}},
			[]string{"-d", "1.0"},
		},
		{
			"long flag uses equals form",
			[]Arg{{Key: "--lossless_jpeg", Value: "1"}},
			[]string{"--lossless_jpeg=1"},
		},
		{
			"valueless flag emits key only",
			[]Arg{{Key: "--progressive", Valueless: true}},
			[]string{"--progressive"},
		},
		{
			"empty key skipped",
			[]Arg{{Key: "", Value: "x"}, {Key: "-e", Value: "7"}},
			[]string{"-e", "7"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Args(tt.args); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Args() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCommand(t *testing.T) {
	got := Command("cjxl", []Arg{{Key: "-d", Value: "0"}, {Key: "-e", Value: "9"}}, "in.png", "out.jxl")
	want := []string{"cjxl", "-d", "0", "-e", "9", "in.png", "out.jxl"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Command() = %v, want %v", got, want)
	}
}
