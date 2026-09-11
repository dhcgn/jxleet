package app

import (
	"context"
	"errors"
	"testing"

	"github.com/dhcgn/jxleet/internal/config"
)

func updateTestService(t *testing.T, version string, cb Callbacks) *Service {
	t.Helper()
	return New(config.Paths{}, config.Default(), nil, version, cb)
}

func TestGetAppUpdateReportsAvailable(t *testing.T) {
	service := updateTestService(t, "v1.0.0", Callbacks{
		CheckAppUpdate: func(context.Context) (Update, error) {
			return Update{
				Latest:    "v9.9.9",
				URL:       "https://github.com/dhcgn/jxleet/releases/tag/v9.9.9",
				Available: true,
			}, nil
		},
	})

	update := service.GetAppUpdate()
	if !update.Available {
		t.Fatal("expected update to be available")
	}
	if update.Latest != "v9.9.9" {
		t.Errorf("Latest = %q, want %q", update.Latest, "v9.9.9")
	}
	if update.URL != "https://github.com/dhcgn/jxleet/releases/tag/v9.9.9" {
		t.Errorf("URL = %q", update.URL)
	}
	if update.Current != "v1.0.0" {
		t.Errorf("Current = %q, want %q", update.Current, "v1.0.0")
	}
}

func TestGetAppUpdateUpToDate(t *testing.T) {
	service := updateTestService(t, "v9.9.9", Callbacks{
		// Silent check found nothing newer.
		CheckAppUpdate: func(context.Context) (Update, error) {
			return Update{}, nil
		},
	})

	if update := service.GetAppUpdate(); update.Available {
		t.Error("up-to-date check must not report an update")
	}
}

func TestGetAppUpdateHidesErrors(t *testing.T) {
	// Offline, rate-limited or otherwise failing checks never surface in
	// the GUI: they report "nothing available" instead of an error.
	service := updateTestService(t, "v1.0.0", Callbacks{
		CheckAppUpdate: func(context.Context) (Update, error) {
			return Update{}, errors.New("offline")
		},
	})

	if update := service.GetAppUpdate(); update.Available {
		t.Error("failing check must not report an update")
	}
}

func TestGetAppUpdateSkipsDevBuild(t *testing.T) {
	// Dev builds return "nothing available" without touching the network,
	// even with a check wired in.
	service := updateTestService(t, "dev", Callbacks{
		CheckAppUpdate: func(context.Context) (Update, error) {
			return Update{Latest: "v9.9.9", Available: true}, nil
		},
	})

	if update := service.GetAppUpdate(); update.Available {
		t.Error("dev build must not report an update")
	}
}

func TestGetAppUpdateWithoutCheck(t *testing.T) {
	// No check wired in (updater init failed or skipped): nothing
	// available, never an error.
	service := updateTestService(t, "v1.0.0", Callbacks{})

	if update := service.GetAppUpdate(); update.Available {
		t.Error("missing check must not report an update")
	}
}

func TestCheckForAppUpdateTriggersInstall(t *testing.T) {
	installed := false
	service := updateTestService(t, "v1.0.0", Callbacks{
		InstallAppUpdate: func(context.Context) error {
			installed = true
			return nil
		},
	})

	if err := service.CheckForAppUpdate(); err != nil {
		t.Fatalf("CheckForAppUpdate: %v", err)
	}
	if !installed {
		t.Error("CheckForAppUpdate must run the install flow")
	}
}

func TestCheckForAppUpdateWithoutInstaller(t *testing.T) {
	service := updateTestService(t, "v1.0.0", Callbacks{})

	if err := service.CheckForAppUpdate(); err == nil {
		t.Error("expected an error when no installer is wired in")
	}
}
