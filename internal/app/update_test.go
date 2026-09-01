package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dhcgn/jxleet/internal/config"
)

func TestLatestAppRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.UserAgent() == "" {
			t.Error("GitHub API requires a User-Agent header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v9.9.9","html_url":"https://github.com/dhcgn/jxleet/releases/tag/v9.9.9"}`))
	}))
	defer server.Close()

	version, url, err := latestAppRelease(context.Background(), server.Client(), server.URL)
	if err != nil {
		t.Fatalf("latestAppRelease: %v", err)
	}
	if version != "v9.9.9" || url != "https://github.com/dhcgn/jxleet/releases/tag/v9.9.9" {
		t.Errorf("latestAppRelease = %q, %q", version, url)
	}
}

func TestGetAppUpdateSkipsDevBuild(t *testing.T) {
	// Dev builds return "nothing available" without touching the network.
	service := New(config.Paths{}, config.Default(), nil, "dev", Callbacks{})
	if update := service.GetAppUpdate(); update.Available {
		t.Error("dev build must not report an update")
	}
}
