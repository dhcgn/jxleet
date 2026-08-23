package cli

import "testing"

func TestParsePathsAndPreset(t *testing.T) {
	got, err := Parse([]string{"a.jpg", "--preset", "web", "folder"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Preset != "web" || len(got.Paths) != 2 {
		t.Fatalf("parsed = %+v", got)
	}
}

func TestParsePresetEquals(t *testing.T) {
	got, err := Parse([]string{"--preset=archive-lossless"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Preset != "archive-lossless" {
		t.Fatalf("preset = %q", got.Preset)
	}
}

func TestParseRejectsUnknownAndIncompleteOptions(t *testing.T) {
	for _, input := range [][]string{{"--nope"}, {"--preset"}, {"--preset="}, {"--register-context-menu", "--unregister-context-menu"}} {
		if _, err := Parse(input); err == nil {
			t.Errorf("Parse(%v) should fail", input)
		}
	}
}

func TestParseHelpAndVersion(t *testing.T) {
	help, err := Parse([]string{"--help"})
	if err != nil || !help.Help {
		t.Fatalf("help = %+v err=%v", help, err)
	}
	version, err := Parse([]string{"--version"})
	if err != nil || !version.Version {
		t.Fatalf("version = %+v err=%v", version, err)
	}
}
