package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestIsPortableExe(t *testing.T) {
	cases := []struct {
		exe  string
		want bool
	}{
		{`C:\tools\jxleet-portable.exe`, true},
		{`C:\tools\JXLEET-PORTABLE.EXE`, true},
		{`C:\tools\jxleet.exe`, false},
		{`C:\portable-tools\jxleet.exe`, false}, // directory name does not count
		{`jxleet.exe`, false},
	}
	for _, c := range cases {
		if got := IsPortableExe(c.exe); got != c.want {
			t.Errorf("IsPortableExe(%q) = %v, want %v", c.exe, got, c.want)
		}
	}
}

func TestResolvePortablePathsStayNextToExe(t *testing.T) {
	exeDir := filepath.Join(t.TempDir(), "usb")
	p := ResolvePortablePaths(exeDir)
	root := filepath.Join(exeDir, "jxleet-files")
	for _, got := range []string{p.ConfigDir, p.PresetsDir, p.BinDir, p.LogsDir} {
		if !strings.HasPrefix(got, root+string(filepath.Separator)) && got != root {
			t.Errorf("portable path %q escapes %q", got, root)
		}
	}
	if p.ConfigFile != filepath.Join(root, "config.yaml") {
		t.Errorf("ConfigFile = %q", p.ConfigFile)
	}
	if err := p.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}
}
