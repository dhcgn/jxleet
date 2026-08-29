package toolchain

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"0.12.0", "0.12.0", 0},
		{"v0.12.1", "0.12.0", 1},
		{"0.11.9", "0.12.0", -1},
		{"1.0.0-beta.1", "1.0.0", -1},
		{"1.0.0-beta.10", "1.0.0-beta.2", 1},
		{"1.0.0+build.2", "1.0.0+build.1", 0},
	}
	for _, tt := range tests {
		got, err := CompareVersions(tt.a, tt.b)
		if err != nil {
			t.Fatalf("CompareVersions(%q, %q): %v", tt.a, tt.b, err)
		}
		if got != tt.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
	if _, err := CompareVersions("not-a-version", "0.12.0"); err == nil {
		t.Error("invalid version should fail")
	}
}

func TestGitHubLatestSelectsStaticAsset(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/libjxl/libjxl/releases/latest" {
			t.Fatalf("unexpected request path %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tag_name":     "v0.12.0",
			"published_at": "2026-07-01T21:16:30Z",
			"assets": []map[string]any{
				{
					"name":                 "jxl-x64-windows-static.zip",
					"browser_download_url": "https://github.com/libjxl/libjxl/releases/download/v0.12.0/jxl-x64-windows-static.zip",
					"digest":               "sha256:" + strings.Repeat("a", 64),
					"size":                 42,
				},
				{"name": "other.zip", "size": 1},
			},
		})
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	client := NewGitHubClient()
	client.BaseURL = server.URL
	release, err := client.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if release.Version != "0.12.0" || release.Asset.Name != windowsAsset || release.Asset.Size != 42 {
		t.Fatalf("unexpected release: %+v", release)
	}
}

func TestGitHubLatestRejectsMissingDigest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tag_name": "v0.12.0",
			"assets": []map[string]any{{
				"name":                 windowsAsset,
				"browser_download_url": "https://example.test/jxl.zip",
				"size":                 1,
			}},
		})
	}))
	defer server.Close()

	client := NewGitHubClient()
	client.BaseURL = server.URL
	if _, err := client.Latest(context.Background()); err == nil {
		t.Fatal("missing digest should fail")
	}
}

func TestDownloadVerifiesChecksumAndSize(t *testing.T) {
	payload := []byte("zip payload")
	sum := sha256.Sum256(payload)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/download.zip" {
			t.Fatalf("unexpected download path %s", r.URL.Path)
		}
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	client := NewGitHubClient()
	manager := NewManagerWithClient(t.TempDir(), client)
	release := Release{
		Version: "0.12.0",
		Asset: Asset{
			Name:        windowsAsset,
			DownloadURL: server.URL + "/download.zip",
			Digest:      "sha256:" + hex.EncodeToString(sum[:]),
			Size:        int64(len(payload)),
		},
	}
	path, err := manager.download(context.Background(), release)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("download = %q, want %q", got, payload)
	}

	release.Asset.Digest = "sha256:" + strings.Repeat("b", 64)
	if _, err := manager.download(context.Background(), release); err == nil {
		t.Fatal("checksum mismatch should fail")
	}
}

func TestExtractZipRejectsSymlinkAndKeepsPathsContained(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "archive.zip")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(file)
	entry, err := zw.Create("../outside.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = entry.Write([]byte("safe"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	destination := filepath.Join(t.TempDir(), "staging")
	if err := os.MkdirAll(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := extractZip(context.Background(), archivePath, destination); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(destination), "outside.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Error("archive entry escaped staging directory")
	}

	symlinkArchive := filepath.Join(t.TempDir(), "symlink.zip")
	symlinkFile, err := os.Create(symlinkArchive)
	if err != nil {
		t.Fatal(err)
	}
	symlinkWriter := zip.NewWriter(symlinkFile)
	header := &zip.FileHeader{Name: "link"}
	header.SetMode(os.ModeSymlink)
	linkEntry, err := symlinkWriter.CreateHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = linkEntry.Write([]byte("../outside"))
	if err := symlinkWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := symlinkFile.Close(); err != nil {
		t.Fatal(err)
	}
	if err := extractZip(context.Background(), symlinkArchive, filepath.Join(t.TempDir(), "staging")); err == nil {
		t.Fatal("symlink archive entry should be rejected")
	}
}

func TestCurrentPointerRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bin", currentFileName)
	if err := writeCurrentVersion(path, "0.12.0"); err != nil {
		t.Fatal(err)
	}
	got, err := readCurrentVersion(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "0.12.0" {
		t.Fatalf("current version = %q", got)
	}
	if _, err := readCurrentVersion(filepath.Join(t.TempDir(), "missing")); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("missing pointer error = %v, want ErrNotInstalled", err)
	}
}

func TestRealArchiveInstall(t *testing.T) {
	archivePath := os.Getenv("JXLEET_TOOLCHAIN_ZIP")
	if archivePath == "" {
		t.Skip("set JXLEET_TOOLCHAIN_ZIP to run the real libjxl archive install")
	}
	info, err := os.Stat(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	sum, err := fileSHA256(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(t.TempDir())
	release := Release{
		TagName: "v0.12.0",
		Version: "0.12.0",
		Asset: Asset{
			Name:        windowsAsset,
			DownloadURL: "https://example.test/not-used",
			Digest:      "sha256:" + sum,
			Size:        info.Size(),
		},
	}
	installed, err := manager.installArchive(context.Background(), release, archivePath)
	if err != nil {
		t.Fatal(err)
	}
	if installed.Version != "0.12.0" {
		t.Fatalf("installed version = %s", installed.Version)
	}
	if _, err := os.Stat(manager.CurrentPath()); err != nil {
		t.Fatal(err)
	}

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tag_name":     "v0.12.0",
			"published_at": "2026-07-01T21:16:30Z",
			"assets": []map[string]any{{
				"name":                 windowsAsset,
				"browser_download_url": "https://example.test/jxl.zip",
				"digest":               "sha256:" + sum,
				"size":                 info.Size(),
			}},
		})
	}))
	defer api.Close()
	manager.Client.BaseURL = api.URL

	status, err := manager.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Installed == nil || status.Installed.CJXLVersion != "0.12.0" || status.Installed.DJXLVersion != "0.12.0" {
		t.Fatalf("unexpected installed status: %+v", status.Installed)
	}
	if status.NeedsInstall || status.UpdateAvailable || status.Flags.Locked {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func fileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
