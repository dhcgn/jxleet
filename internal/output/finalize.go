package output

import (
	"context"
	"fmt"
	"os"

	"github.com/dhcgn/jxleet/internal/preset"
	"github.com/dhcgn/jxleet/internal/routes"
)

// Verifier decodes a freshly written .jxl to prove it is valid before the
// original is ever touched. It is implemented by the djxl package.
type Verifier interface {
	// Readable returns nil if the .jxl decodes successfully.
	Readable(ctx context.Context, jxlPath string) error
	// Reconstructs returns nil if the .jxl decodes back to a JPEG that is
	// byte-identical to originalJPEG (transcode route only).
	Reconstructs(ctx context.Context, jxlPath, originalJPEG string) error
}

// FinalizeOptions controls how a written temp file is committed to its final
// location.
type FinalizeOptions struct {
	Route routes.Route
	// Verifier proves the temp .jxl is valid. It must be set for the replace
	// policy; a nil Verifier skips verification (only sensible for tests or
	// non-destructive policies).
	Verifier Verifier
	// OriginalJPEG is the source JPEG path; when set and Route is Transcode, the
	// result is checked to reconstruct it byte for byte.
	OriginalJPEG string

	// Test hooks; nil means the real implementations are used.
	recycle      func(string) error
	recycleCheck func(string) bool
}

// Finalize commits the encoder's temp output (plan.TempPath) to plan.Final,
// following the safe order for the replace policy: verify the result, move it
// into place, and only then send the original to the recycle bin. On any failure
// the original is left recoverable and the temp file is cleaned up.
func Finalize(ctx context.Context, plan Plan, opt FinalizeOptions) error {
	if plan.Skip {
		return nil
	}
	recycle := opt.recycle
	if recycle == nil {
		recycle = MoveToRecycleBin
	}
	recycleCheck := opt.recycleCheck
	if recycleCheck == nil {
		recycleCheck = RecycleBinAvailable
	}

	// 1. Verify the result before touching anything else.
	if opt.Verifier != nil {
		if err := opt.Verifier.Readable(ctx, plan.TempPath); err != nil {
			os.Remove(plan.TempPath)
			return err
		}
		if opt.Route == routes.RouteTranscode && opt.OriginalJPEG != "" {
			if err := opt.Verifier.Reconstructs(ctx, plan.TempPath, opt.OriginalJPEG); err != nil {
				os.Remove(plan.TempPath)
				return err
			}
		}
	}

	if plan.Policy != preset.PolicyReplace {
		// alongside / subfolder: just move into place.
		if err := os.Rename(plan.TempPath, plan.Final); err != nil {
			os.Remove(plan.TempPath)
			return fmt.Errorf("output: move result into place: %w", err)
		}
		return nil
	}

	// Replace policy: the recycle bin is mandatory; refuse rather than risk a
	// permanent delete on a volume without one.
	if !recycleCheck(plan.Input) {
		os.Remove(plan.TempPath)
		return fmt.Errorf("output: refusing to replace %s: the volume has no recycle bin", plan.Input)
	}

	if plan.InPlace {
		return finalizeInPlace(plan, recycle)
	}
	return finalizeReplace(plan, recycle)
}

// finalizeReplace handles replace when the result path differs from the input
// (e.g. photo.jpg -> photo.jxl): move result into place, then recycle original.
func finalizeReplace(plan Plan, recycle func(string) error) error {
	if err := os.Rename(plan.TempPath, plan.Final); err != nil {
		os.Remove(plan.TempPath)
		return fmt.Errorf("output: move result into place: %w", err)
	}
	if err := recycle(plan.Input); err != nil {
		// The result is in place; the original remains on disk, just not
		// recycled. Surface the error so the caller can report it.
		return fmt.Errorf("output: result written but original could not be recycled: %w", err)
	}
	return nil
}

// finalizeInPlace handles replace when the result path equals the input (e.g.
// reencoding photo.jxl onto itself): move the original aside first so it is
// never overwritten before the new file is safely in place.
func finalizeInPlace(plan Plan, recycle func(string) error) error {
	backup := plan.Input + ".jxleet-bak"
	if err := os.Rename(plan.Input, backup); err != nil {
		os.Remove(plan.TempPath)
		return fmt.Errorf("output: set aside original: %w", err)
	}
	if err := os.Rename(plan.TempPath, plan.Final); err != nil {
		// Roll back: restore the original.
		_ = os.Rename(backup, plan.Input)
		os.Remove(plan.TempPath)
		return fmt.Errorf("output: move result into place: %w", err)
	}
	if err := recycle(backup); err != nil {
		return fmt.Errorf("output: result written but original could not be recycled (left at %s): %w", backup, err)
	}
	return nil
}

// NeedsConfirmation reports whether committing this file requires the
// irreversible-replace confirmation: the original will be recycled and the
// result cannot be reversed back to it.
func NeedsConfirmation(policy preset.Policy, route routes.Route, distance float64) bool {
	return policy == preset.PolicyReplace && !route.Reversible(distance)
}
