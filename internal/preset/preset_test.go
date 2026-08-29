package preset

import (
	"testing"

	"github.com/dhcgn/jxleet/internal/cjxl/flags"
	"github.com/dhcgn/jxleet/internal/routes"
)

const sampleYAML = `name: archive-lossless
description: keep jpeg recoverable
version: 1
output:
  policy: alongside
  on_collision: skip
rules:
  - match: [JPEG]
    args:
      "--lossless_jpeg": 1
  - match: [PNG, EXR]
    args:
      "-d": 0
      "-e": 9
      "--num_threads": 8
      "--progressive": true
      "--noise": false
  - match: ["*"]
    args:
      "-d": 1.0
      "-e": 7
`

func TestUnmarshalOrderAndFlags(t *testing.T) {
	p, err := Unmarshal([]byte(sampleYAML))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Rules) != 3 {
		t.Fatalf("got %d rules, want 3", len(p.Rules))
	}

	// Rule 1 must preserve arg order and drop the `false` flag.
	got := p.Rules[1].Args
	want := []struct {
		key       string
		value     string
		valueless bool
	}{
		{"-d", "0", false},
		{"-e", "9", false},
		{"--num_threads", "8", false},
		{"--progressive", "", true},
	}
	if len(got) != len(want) {
		t.Fatalf("rule 1 has %d args, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Key != w.key || got[i].Value != w.value || got[i].Valueless != w.valueless {
			t.Errorf("arg %d = %+v, want %v", i, got[i], w)
		}
	}
}

func TestMarshalRoundTrip(t *testing.T) {
	p, err := Unmarshal([]byte(sampleYAML))
	if err != nil {
		t.Fatal(err)
	}
	data, err := Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("re-parse: %v\n%s", err, data)
	}
	// The float value must survive as "1.0", not become "1".
	last := p2.Rules[2].Args
	if last[0].Key != "-d" || last[0].Value != "1.0" {
		t.Errorf("distance did not round-trip: %+v", last[0])
	}
	// Order and valueless flags must survive.
	r1 := p2.Rules[1].Args
	if len(r1) != 4 || r1[3].Key != "--progressive" || !r1[3].Valueless {
		t.Errorf("rule 1 did not round-trip: %+v", r1)
	}
}

func TestRoute(t *testing.T) {
	p, err := Unmarshal([]byte(sampleYAML))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		format routes.Format
		want   routes.Route
	}{
		{routes.FormatJPEG, routes.RouteTranscode}, // --lossless_jpeg defaults/set to 1
		{routes.FormatPNG, routes.RouteEncode},
		{routes.FormatEXR, routes.RouteEncode},
		{routes.FormatJXL, routes.RouteReencode}, // matches "*"
	}
	for _, c := range cases {
		route, args, ok := p.Route(c.format)
		if !ok {
			t.Errorf("%s: no rule matched", c.format)
			continue
		}
		if route != c.want {
			t.Errorf("%s: route = %v, want %v", c.format, route, c.want)
		}
		if len(args) == 0 {
			t.Errorf("%s: expected resolved args", c.format)
		}
	}
}

func TestRouteJPEGReencodeWhenLossy(t *testing.T) {
	y := `name: web
version: 1
output: {policy: replace, on_collision: overwrite}
rules:
  - match: ["*"]
    args:
      "--lossless_jpeg": 0
      "-d": 1.5
`
	p, err := Unmarshal([]byte(y))
	if err != nil {
		t.Fatal(err)
	}
	route, _, ok := p.Route(routes.FormatJPEG)
	if !ok || route != routes.RouteReencode {
		t.Errorf("JPEG with --lossless_jpeg=0 should reencode, got %v ok=%v", route, ok)
	}
}

func TestValidate(t *testing.T) {
	good, _ := Unmarshal([]byte(sampleYAML))
	if err := good.Validate(); err != nil {
		t.Errorf("valid preset rejected: %v", err)
	}

	bad := good
	bad.Name = ""
	if err := bad.Validate(); err == nil {
		t.Error("empty name should fail")
	}

	sub := good
	sub.Output = Output{Policy: PolicySubfolder}
	if err := sub.Validate(); err == nil {
		t.Error("subfolder policy without subfolder should fail")
	}
}

func TestValidateArgs(t *testing.T) {
	p, _ := Unmarshal([]byte(sampleYAML))
	if err := p.ValidateArgs(flags.Default()); err != nil {
		t.Errorf("sample args should be valid: %v", err)
	}

	bad := `name: b
version: 1
output: {policy: alongside}
rules:
  - match: ["*"]
    args:
      "--not-a-real-flag": 1
`
	bp, _ := Unmarshal([]byte(bad))
	if err := bp.ValidateArgs(flags.Default()); err == nil {
		t.Error("unknown flag should be rejected")
	}
}

func TestMigrate(t *testing.T) {
	p := Preset{Name: "x", Version: 0}
	m, err := migrate(p)
	if err != nil || m.Version != CurrentVersion {
		t.Errorf("version 0 should migrate to %d, got %d err=%v", CurrentVersion, m.Version, err)
	}
	if _, err := migrate(Preset{Name: "y", Version: 999}); err == nil {
		t.Error("future version should be rejected")
	}
}
