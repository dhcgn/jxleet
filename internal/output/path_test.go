package output

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dhcgn/jxleet/internal/preset"
)

func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareAlongside(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "photo.jpg")
	plan, err := Prepare(in, preset.Output{Policy: preset.PolicyAlongside, OnCollision: preset.CollisionSkip})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Final != filepath.Join(dir, "photo.jxl") {
		t.Errorf("final = %s", plan.Final)
	}
	if plan.InPlace {
		t.Error("jpg->jxl is not in place")
	}
	if filepath.Dir(plan.TempPath) != dir {
		t.Errorf("temp must be in the target dir: %s", plan.TempPath)
	}
}

func TestPrepareSubfolderCreatesDir(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "photo.png")
	plan, err := Prepare(in, preset.Output{Policy: preset.PolicySubfolder, Subfolder: "jxl"})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "jxl", "photo.jxl")
	if plan.Final != want {
		t.Errorf("final = %s, want %s", plan.Final, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "jxl")); err != nil {
		t.Errorf("subfolder should have been created: %v", err)
	}
}

func TestPrepareReplaceInPlace(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "photo.jxl")
	touch(t, in)
	plan, err := Prepare(in, preset.Output{Policy: preset.PolicyReplace})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.InPlace {
		t.Error("jxl->jxl replace should be in place")
	}
	if plan.Skip {
		t.Error("in-place replace must not be treated as a collision")
	}
	if !samePath(plan.Final, in) {
		t.Errorf("final %s should equal input %s", plan.Final, in)
	}
}

func TestPrepareCollision(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "photo.jpg")
	touch(t, filepath.Join(dir, "photo.jxl")) // pre-existing output

	skip, _ := Prepare(in, preset.Output{Policy: preset.PolicyAlongside, OnCollision: preset.CollisionSkip})
	if !skip.Skip {
		t.Error("skip mode should skip on collision")
	}

	num, err := Prepare(in, preset.Output{Policy: preset.PolicyAlongside, OnCollision: preset.CollisionNumber})
	if err != nil {
		t.Fatal(err)
	}
	if num.Final != filepath.Join(dir, "photo (1).jxl") {
		t.Errorf("numbered final = %s", num.Final)
	}

	over, err := Prepare(in, preset.Output{Policy: preset.PolicyAlongside, OnCollision: preset.CollisionOverwrite})
	if err != nil {
		t.Fatal(err)
	}
	if over.Final != filepath.Join(dir, "photo.jxl") || over.Skip {
		t.Errorf("overwrite should keep the original name, got %s skip=%v", over.Final, over.Skip)
	}
}
