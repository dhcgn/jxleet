package output

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/dhcgn/jxleet/internal/preset"
	"github.com/dhcgn/jxleet/internal/routes"
)

type fakeVerifier struct {
	readErr    error
	reconErr   error
	readCalls  int
	reconCalls int
}

func (f *fakeVerifier) Readable(_ context.Context, _ string) error {
	f.readCalls++
	return f.readErr
}

func (f *fakeVerifier) Reconstructs(_ context.Context, _, _ string) error {
	f.reconCalls++
	return f.reconErr
}

// makeTemp writes a temp file at plan.TempPath with the given content, mimicking
// the encoder having produced output.
func makeTemp(t *testing.T, plan Plan, content string) {
	t.Helper()
	if err := os.WriteFile(plan.TempPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFinalizeAlongside(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "photo.jpg")
	touch(t, in)
	plan, err := Prepare(in, preset.Output{Policy: preset.PolicyAlongside})
	if err != nil {
		t.Fatal(err)
	}
	makeTemp(t, plan, "result")

	v := &fakeVerifier{}
	if err := Finalize(context.Background(), plan, FinalizeOptions{Route: routes.RouteEncode, Verifier: v}); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(plan.Final); string(data) != "result" {
		t.Errorf("final content = %q", data)
	}
	if _, err := os.Stat(plan.TempPath); !os.IsNotExist(err) {
		t.Error("temp should be gone")
	}
	if _, err := os.Stat(in); err != nil {
		t.Error("alongside must not touch the original")
	}
}

func TestFinalizeReplaceRecyclesOriginal(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "photo.jpg")
	touch(t, in)
	plan, err := Prepare(in, preset.Output{Policy: preset.PolicyReplace})
	if err != nil {
		t.Fatal(err)
	}
	makeTemp(t, plan, "result")

	var recycled string
	opt := FinalizeOptions{
		Route:        routes.RouteReencode,
		Verifier:     &fakeVerifier{},
		recycle:      func(p string) error { recycled = p; return os.Remove(p) },
		recycleCheck: func(string) bool { return true },
	}
	if err := Finalize(context.Background(), plan, opt); err != nil {
		t.Fatal(err)
	}
	if !samePath(recycled, in) {
		t.Errorf("recycled %q, want original %q", recycled, in)
	}
	if data, _ := os.ReadFile(plan.Final); string(data) != "result" {
		t.Errorf("final content = %q", data)
	}
	if _, err := os.Stat(in); !os.IsNotExist(err) {
		t.Error("original should have been recycled")
	}
}

func TestFinalizeReplaceInPlace(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "photo.jxl")
	if err := os.WriteFile(in, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan, err := Prepare(in, preset.Output{Policy: preset.PolicyReplace})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.InPlace {
		t.Fatal("expected in-place")
	}
	makeTemp(t, plan, "new")

	var recycled string
	opt := FinalizeOptions{
		Route:        routes.RouteReencode,
		Verifier:     &fakeVerifier{},
		recycle:      func(p string) error { recycled = p; return os.Remove(p) },
		recycleCheck: func(string) bool { return true },
	}
	if err := Finalize(context.Background(), plan, opt); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(plan.Final); string(data) != "new" {
		t.Errorf("final content = %q, want new", data)
	}
	if recycled == "" {
		t.Error("the old original should have been recycled via a backup")
	}
}

func TestFinalizeRefusesWithoutRecycleBin(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "photo.jpg")
	touch(t, in)
	plan, err := Prepare(in, preset.Output{Policy: preset.PolicyReplace})
	if err != nil {
		t.Fatal(err)
	}
	makeTemp(t, plan, "result")

	opt := FinalizeOptions{
		Route:        routes.RouteReencode,
		Verifier:     &fakeVerifier{},
		recycle:      func(string) error { t.Fatal("recycle must not be called"); return nil },
		recycleCheck: func(string) bool { return false },
	}
	if err := Finalize(context.Background(), plan, opt); err == nil {
		t.Fatal("expected refusal when no recycle bin")
	}
	if _, err := os.Stat(in); err != nil {
		t.Error("original must be untouched")
	}
	if _, err := os.Stat(plan.TempPath); !os.IsNotExist(err) {
		t.Error("temp should be cleaned up")
	}
}

func TestFinalizeVerifyFailureLeavesOriginal(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "photo.jpg")
	touch(t, in)
	plan, err := Prepare(in, preset.Output{Policy: preset.PolicyReplace})
	if err != nil {
		t.Fatal(err)
	}
	makeTemp(t, plan, "result")

	opt := FinalizeOptions{
		Route:        routes.RouteReencode,
		Verifier:     &fakeVerifier{readErr: errors.New("corrupt")},
		recycle:      func(string) error { t.Fatal("must not recycle on verify failure"); return nil },
		recycleCheck: func(string) bool { return true },
	}
	if err := Finalize(context.Background(), plan, opt); err == nil {
		t.Fatal("expected verification error")
	}
	if _, err := os.Stat(in); err != nil {
		t.Error("original must survive a verification failure")
	}
	if _, err := os.Stat(plan.Final); err == nil {
		t.Error("final must not be created on verification failure")
	}
}

func TestFinalizeTranscodeReconstruction(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "photo.jpg")
	touch(t, in)
	plan, err := Prepare(in, preset.Output{Policy: preset.PolicyReplace})
	if err != nil {
		t.Fatal(err)
	}
	makeTemp(t, plan, "result")

	v := &fakeVerifier{}
	opt := FinalizeOptions{
		Route:        routes.RouteTranscode,
		Verifier:     v,
		OriginalJPEG: in,
		recycle:      func(p string) error { return os.Remove(p) },
		recycleCheck: func(string) bool { return true },
	}
	if err := Finalize(context.Background(), plan, opt); err != nil {
		t.Fatal(err)
	}
	if v.reconCalls != 1 {
		t.Errorf("reconstruction check should run once on the transcode route, ran %d", v.reconCalls)
	}
}

func TestNeedsConfirmation(t *testing.T) {
	if !NeedsConfirmation(preset.PolicyReplace, routes.RouteReencode, 1.5) {
		t.Error("replace + reencode is irreversible")
	}
	if NeedsConfirmation(preset.PolicyReplace, routes.RouteTranscode, 0) {
		t.Error("replace + transcode is reversible, no confirmation needed")
	}
	if NeedsConfirmation(preset.PolicyAlongside, routes.RouteReencode, 1.5) {
		t.Error("alongside never needs the replace confirmation")
	}
}

func TestEffectiveOutputDeletionKeep(t *testing.T) {
	del := DeletionByRoute{routes.RouteTranscode: DeletionKeep}
	got := EffectiveOutput(preset.Output{Policy: preset.PolicyReplace}, routes.RouteTranscode, del)
	if got.Policy != preset.PolicyAlongside {
		t.Errorf("keep rule should downgrade replace to alongside, got %s", got.Policy)
	}
	got = EffectiveOutput(preset.Output{Policy: preset.PolicyReplace}, routes.RouteReencode, del)
	if got.Policy != preset.PolicyReplace {
		t.Errorf("default (recycle) route should stay replace, got %s", got.Policy)
	}
}
