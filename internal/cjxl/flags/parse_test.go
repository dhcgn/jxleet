package flags

import (
	"os"
	"strings"
	"testing"
)

func TestParseTestdata(t *testing.T) {
	f, err := os.Open("testdata/cjxl-help-0.12.0.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	parsed, err := Parse(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed) < 50 {
		t.Fatalf("expected many flags, got %d", len(parsed))
	}

	set := NewSet("0.12.0", parsed)

	cases := []struct {
		token      string
		wantShort  string
		wantLong   string
		takesValue bool
		valueSpec  string
	}{
		{"--distance", "d", "distance", true, "DISTANCE"},
		{"-d", "d", "distance", true, "DISTANCE"},
		{"--lossless_jpeg", "j", "lossless_jpeg", true, "0|1"},
		{"-j", "j", "lossless_jpeg", true, "0|1"},
		{"--quiet", "", "quiet", false, ""},
		{"--progressive_ac", "", "progressive_ac", false, ""},
		{"--dec-hints", "x", "dec-hints", true, "key=value"},
		{"--modular_colorspace", "C", "modular_colorspace", true, "-1..41"},
		{"--num_threads", "", "num_threads", true, "THREADS"},
	}
	for _, c := range cases {
		t.Run(c.token, func(t *testing.T) {
			f, ok := set.Lookup(c.token)
			if !ok {
				t.Fatalf("token %q not found", c.token)
			}
			if f.Short != c.wantShort || f.Long != c.wantLong {
				t.Errorf("names = -%s/--%s, want -%s/--%s", f.Short, f.Long, c.wantShort, c.wantLong)
			}
			if f.TakesValue != c.takesValue {
				t.Errorf("takesValue = %v, want %v", f.TakesValue, c.takesValue)
			}
			if f.ValueSpec != c.valueSpec {
				t.Errorf("valueSpec = %q, want %q", f.ValueSpec, c.valueSpec)
			}
		})
	}
}

func TestParseIgnoresNoise(t *testing.T) {
	// The INPUT/OUTPUT block, the Usage banner and an unindented description
	// line must not become flags.
	help := `Usage: cjxl.exe INPUT OUTPUT [OPTIONS...]
 INPUT
    the input can be JXL, PPM
 OUTPUT
    the compressed JXL output file

Basic options:
 -d DISTANCE, --distance=DISTANCE
    Target visual distance.
Mutually exclusive with --quality.
 --quiet
    Minimal printing.
`
	parsed, err := Parse(strings.NewReader(help))
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed) != 2 {
		t.Fatalf("got %d flags, want 2: %+v", len(parsed), parsed)
	}
	if parsed[0].Long != "distance" || parsed[1].Long != "quiet" {
		t.Errorf("unexpected flags: %+v", parsed)
	}
	if parsed[0].Section != "Basic options" {
		t.Errorf("section = %q, want %q", parsed[0].Section, "Basic options")
	}
}

func TestValidate(t *testing.T) {
	set := Default()
	for _, ok := range []string{"-d", "--distance", "--lossless_jpeg", "-j", "--progressive"} {
		if err := set.Validate(ok); err != nil {
			t.Errorf("Validate(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"--nope", "-Z", "--losless_jpeg"} {
		if err := set.Validate(bad); err == nil {
			t.Errorf("Validate(%q) = nil, want error", bad)
		}
	}
}

func TestDefaultMatchesTestdata(t *testing.T) {
	// The committed generated set must stay in sync with the captured help.
	f, err := os.Open("testdata/cjxl-help-0.12.0.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	parsed, err := Parse(f)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(Default().Flags), len(parsed); got != want {
		t.Errorf("Default() has %d flags, testdata parses to %d — run go generate", got, want)
	}
}

func TestDiff(t *testing.T) {
	old := NewSet("0.11", []Flag{{Long: "a"}, {Long: "b"}})
	updated := NewSet("0.12", []Flag{{Long: "b"}, {Long: "c"}})
	added, removed := Diff(old, updated)
	if len(added) != 1 || added[0] != "--c" {
		t.Errorf("added = %v, want [--c]", added)
	}
	if len(removed) != 1 || removed[0] != "--a" {
		t.Errorf("removed = %v, want [--a]", removed)
	}
}
