package toolchain

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dhcgn/jxleet/internal/process"
	"github.com/spf13/fileflow"
	"github.com/spf13/pathologize"
)

const (
	maxDownloadBytes     = 512 << 20
	maxArchiveEntryBytes = 512 << 20
	maxArchiveBytes      = 1 << 30
)

// InstallRelease downloads, verifies, extracts and activates a release. The
// active pointer is changed only after all three executables are present and
// their versions have been verified.
func (m *Manager) InstallRelease(ctx context.Context, release Release) (Installation, error) {
	if err := validateRelease(release); err != nil {
		return Installation{}, err
	}
	if m.BinDir == "" {
		return Installation{}, errors.New("toolchain: binary directory is empty")
	}
	if err := os.MkdirAll(m.VersionsDir(), 0o755); err != nil {
		return Installation{}, fmt.Errorf("toolchain: create versions directory: %w", err)
	}

	versionRoot := safeJoin(m.VersionsDir(), release.Version)
	if info, err := os.Stat(versionRoot); err == nil {
		if !info.IsDir() {
			return Installation{}, fmt.Errorf("toolchain: version path is not a directory: %s", versionRoot)
		}
		installed, inspectErr := m.inspectInstallation(ctx, release.Version)
		if inspectErr != nil {
			return Installation{}, fmt.Errorf("toolchain: existing %s install is incomplete: %w", release.Version, inspectErr)
		}
		if err := writeCurrentVersion(m.CurrentPath(), release.Version); err != nil {
			return Installation{}, err
		}
		return installed, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return Installation{}, fmt.Errorf("toolchain: inspect version path: %w", err)
	}

	archivePath, err := m.download(ctx, release)
	if err != nil {
		return Installation{}, err
	}
	defer func() { _ = os.Remove(archivePath) }()

	m.reportProgress(InstallProgress{Phase: "installing"})
	return m.installArchive(ctx, release, archivePath)
}

func (m *Manager) installArchive(ctx context.Context, release Release, archivePath string) (Installation, error) {
	if err := validateRelease(release); err != nil {
		return Installation{}, err
	}
	if err := os.MkdirAll(m.VersionsDir(), 0o755); err != nil {
		return Installation{}, fmt.Errorf("toolchain: create versions directory: %w", err)
	}
	staging, err := makeStagingDir(m.VersionsDir(), release.Version)
	if err != nil {
		return Installation{}, err
	}
	defer func() { _ = os.RemoveAll(staging) }()

	if err := extractZip(ctx, archivePath, staging); err != nil {
		return Installation{}, err
	}
	binaries, err := findBinaries(staging)
	if err != nil {
		return Installation{}, err
	}

	toolsDir := m.ToolsDir(release.Version)
	for _, name := range []string{"cjxl.exe", "djxl.exe", "jxlinfo.exe"} {
		dst := filepath.Join(toolsDir, name)
		landed, err := fileflow.Move(binaries[name], dst)
		if err != nil {
			return Installation{}, fmt.Errorf("toolchain: install %s: %w", name, err)
		}
		if !samePath(landed, dst) {
			return Installation{}, fmt.Errorf("toolchain: %s landed at unexpected path %s", name, landed)
		}
	}

	installed, err := m.inspectInstallation(ctx, release.Version)
	if err != nil {
		return Installation{}, err
	}
	if err := writeCurrentVersion(m.CurrentPath(), release.Version); err != nil {
		return Installation{}, err
	}
	return installed, nil
}

func validateRelease(release Release) error {
	version, err := normalizeVersion(release.Version)
	if err != nil {
		return err
	}
	if version != release.Version {
		return fmt.Errorf("toolchain: release version is not normalized: %q", release.Version)
	}
	if release.Asset.Name != windowsAsset {
		return fmt.Errorf("toolchain: unexpected release asset %q", release.Asset.Name)
	}
	if err := validateDigest(release.Asset.Digest); err != nil {
		return err
	}
	u, err := urlParse(release.Asset.DownloadURL)
	if err != nil {
		return fmt.Errorf("toolchain: invalid release download URL: %w", err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("toolchain: unsupported release download URL scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return errors.New("toolchain: release download URL has no host")
	}
	if release.Asset.Size <= 0 || release.Asset.Size > maxDownloadBytes {
		return fmt.Errorf("toolchain: invalid release asset size %d", release.Asset.Size)
	}
	return nil
}

func (m *Manager) download(ctx context.Context, release Release) (string, error) {
	if err := validateRelease(release); err != nil {
		return "", err
	}
	client := m.Client
	if client == nil {
		client = NewGitHubClient()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, release.Asset.DownloadURL, nil)
	if err != nil {
		return "", fmt.Errorf("toolchain: create download request: %w", err)
	}
	req.Header.Set("User-Agent", client.UserAgent)
	resp, err := client.HTTPClientOrDefault().Do(req)
	if err != nil {
		return "", fmt.Errorf("toolchain: download %s: %w", release.Asset.Name, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("toolchain: download returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	if resp.ContentLength > maxDownloadBytes {
		return "", fmt.Errorf("toolchain: download is too large: %d bytes", resp.ContentLength)
	}

	total := resp.ContentLength
	if total <= 0 {
		total = release.Asset.Size
	}
	m.reportProgress(InstallProgress{Phase: "downloading", Total: total})

	if err := os.MkdirAll(m.BinDir, 0o755); err != nil {
		return "", fmt.Errorf("toolchain: create binary directory: %w", err)
	}
	tmp, err := os.CreateTemp(m.BinDir, ".jxl-download-*.zip")
	if err != nil {
		return "", fmt.Errorf("toolchain: create download temp: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		_ = tmp.Close()
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	hasher := sha256.New()
	body := &progressReader{inner: io.LimitReader(resp.Body, maxDownloadBytes+1), total: total, onRead: m.reportProgress}
	written, err := io.Copy(io.MultiWriter(tmp, hasher), body)
	if err != nil {
		return "", fmt.Errorf("toolchain: write download: %w", err)
	}
	if written > maxDownloadBytes {
		return "", fmt.Errorf("toolchain: download exceeds %d bytes", maxDownloadBytes)
	}
	if release.Asset.Size > 0 && written != release.Asset.Size {
		return "", fmt.Errorf("toolchain: download size %d differs from GitHub size %d", written, release.Asset.Size)
	}
	if err := tmp.Sync(); err != nil {
		return "", fmt.Errorf("toolchain: flush download: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("toolchain: close download: %w", err)
	}
	actual := "sha256:" + hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(actual, release.Asset.Digest) {
		return "", fmt.Errorf("toolchain: checksum mismatch: got %s, want %s", actual, release.Asset.Digest)
	}
	cleanup = false
	return tmpPath, nil
}

func makeStagingDir(versionsDir, version string) (string, error) {
	for attempt := 0; attempt < 10; attempt++ {
		name := fmt.Sprintf("%s-staging-%d", version, time.Now().UnixNano())
		path := safeJoin(versionsDir, name)
		if err := os.Mkdir(path, 0o755); err == nil {
			return path, nil
		} else if !errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("toolchain: create staging directory: %w", err)
		}
		time.Sleep(time.Nanosecond)
	}
	return "", errors.New("toolchain: could not create a unique staging directory")
}

func extractZip(ctx context.Context, archivePath, destination string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("toolchain: open archive: %w", err)
	}
	defer func() { _ = reader.Close() }()

	var totalBytes uint64
	for _, entry := range reader.File {
		if err := validateZipEntry(destination, entry); err != nil {
			return err
		}
		if entry.UncompressedSize64 > maxArchiveBytes-totalBytes {
			return fmt.Errorf("toolchain: archive expands beyond %d bytes", maxArchiveBytes)
		}
		totalBytes += entry.UncompressedSize64
	}
	if err := extractZipWithStdlib(reader.File, destination); err != nil {
		if !errors.Is(err, zip.ErrAlgorithm) {
			return err
		}
		if err := os.RemoveAll(destination); err != nil {
			return fmt.Errorf("toolchain: reset staging directory: %w", err)
		}
		if err := os.MkdirAll(destination, 0o755); err != nil {
			return fmt.Errorf("toolchain: recreate staging directory: %w", err)
		}
		if err := extractZipWithPowerShell(ctx, archivePath, destination); err != nil {
			return err
		}
	}
	return nil
}

func validateZipEntry(destination string, entry *zip.File) error {
	if entry.Name == "" {
		return nil
	}
	if entry.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("toolchain: archive contains symlink %q", entry.Name)
	}
	target := safeJoin(destination, entry.Name)
	if !underRoot(destination, target) {
		return fmt.Errorf("toolchain: archive path escapes staging directory: %q", entry.Name)
	}
	if entry.UncompressedSize64 > maxArchiveEntryBytes {
		return fmt.Errorf("toolchain: archive entry %q is too large", entry.Name)
	}
	return nil
}

func extractZipWithStdlib(entries []*zip.File, destination string) error {
	for _, entry := range entries {
		if entry.Name == "" {
			continue
		}
		target := safeJoin(destination, entry.Name)
		if !underRoot(destination, target) {
			return fmt.Errorf("toolchain: archive path escapes staging directory: %q", entry.Name)
		}
		if entry.FileInfo().IsDir() || strings.HasSuffix(entry.Name, "/") {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("toolchain: create archive directory %q: %w", entry.Name, err)
			}
			continue
		}
		if entry.UncompressedSize64 > maxArchiveEntryBytes {
			return fmt.Errorf("toolchain: archive entry %q is too large", entry.Name)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("toolchain: create archive parent for %q: %w", entry.Name, err)
		}
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
		if err != nil {
			return fmt.Errorf("toolchain: create extracted file %q: %w", entry.Name, err)
		}
		in, err := entry.Open()
		if err != nil {
			_ = out.Close()
			return fmt.Errorf("toolchain: open archive entry %q: %w", entry.Name, err)
		}
		_, copyErr := io.Copy(out, io.LimitReader(in, maxArchiveEntryBytes+1))
		closeInErr := in.Close()
		closeOutErr := out.Close()
		if copyErr != nil {
			return fmt.Errorf("toolchain: extract %q: %w", entry.Name, copyErr)
		}
		if closeInErr != nil {
			return fmt.Errorf("toolchain: close archive entry %q: %w", entry.Name, closeInErr)
		}
		if closeOutErr != nil {
			return fmt.Errorf("toolchain: close extracted file %q: %w", entry.Name, closeOutErr)
		}
		info, err := os.Stat(target)
		if err != nil {
			return fmt.Errorf("toolchain: stat extracted file %q: %w", entry.Name, err)
		}
		if info.Size() > maxArchiveEntryBytes {
			return fmt.Errorf("toolchain: archive entry %q is too large", entry.Name)
		}
	}
	return nil
}

func extractZipWithPowerShell(ctx context.Context, archivePath, destination string) error {
	script := fmt.Sprintf(
		"$ErrorActionPreference = 'Stop'; "+
			"$shell = New-Object -ComObject Shell.Application; "+
			"$zip = $shell.Namespace('%s'); "+
			"$dst = $shell.Namespace('%s'); "+
			"if ($null -eq $zip -or $null -eq $dst) { throw 'could not open archive or destination' }; "+
			"$dst.CopyHere($zip.Items(), 0x14); "+
			"$deadline = [DateTime]::UtcNow.AddMinutes(5); "+
			"while ([DateTime]::UtcNow -lt $deadline) { "+
			"  $names = @(Get-ChildItem -LiteralPath '%s' -Recurse -File -ErrorAction SilentlyContinue | Select-Object -ExpandProperty Name); "+
			"  if ($names -contains 'cjxl.exe' -and $names -contains 'djxl.exe' -and $names -contains 'jxlinfo.exe') { exit 0 }; "+
			"  Start-Sleep -Milliseconds 200 "+
			"}; "+
			"throw 'archive extraction timed out'",
		quotePowerShell(archivePath),
		quotePowerShell(destination),
		quotePowerShell(destination),
	)
	cmd := process.CommandContext(ctx, powershellExecutable(), "-NoProfile", "-NonInteractive", "-STA", "-Command", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("toolchain: extract Deflate64 archive with Windows Shell (%s -> %s): %w: %s", archivePath, destination, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func powershellExecutable() string {
	const legacy = `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`
	if _, err := os.Stat(legacy); err == nil {
		return legacy
	}
	return "powershell.exe"
}

func quotePowerShell(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}

func safeJoin(root string, parts ...string) string {
	return filepath.FromSlash(pathologize.Join(root, parts...))
}

func findBinaries(root string) (map[string]string, error) {
	required := map[string]string{
		"cjxl.exe":    "",
		"djxl.exe":    "",
		"jxlinfo.exe": "",
	}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		name := strings.ToLower(entry.Name())
		if _, ok := required[name]; !ok {
			return nil
		}
		if required[name] != "" {
			return fmt.Errorf("toolchain: archive contains duplicate %s", name)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("toolchain: archive entry %s is not a regular file", name)
		}
		required[name] = path
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("toolchain: locate binaries: %w", err)
	}
	for name, path := range required {
		if path == "" {
			return nil, fmt.Errorf("toolchain: archive does not contain %s", name)
		}
	}
	return required, nil
}

func underRoot(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func samePath(a, b string) bool {
	aa, err1 := filepath.Abs(a)
	bb, err2 := filepath.Abs(b)
	if err1 != nil || err2 != nil {
		return strings.EqualFold(a, b)
	}
	return strings.EqualFold(filepath.Clean(aa), filepath.Clean(bb))
}

func urlParse(raw string) (*url.URL, error) {
	return url.Parse(raw)
}

// reportProgress forwards an install progress update to the manager callback, if
// one is configured. It never blocks the install.
func (m *Manager) reportProgress(p InstallProgress) {
	if m.OnInstallProgress != nil {
		m.OnInstallProgress(p)
	}
}

// progressReader wraps a reader and reports cumulative bytes read to a callback.
// Updates are throttled to at most one per 256 KiB so a large download does not
// flood the event channel.
type progressReader struct {
	inner  io.Reader
	total  int64
	read   int64
	last   int64
	onRead func(InstallProgress)
}

const progressReportInterval = 256 << 10

func (r *progressReader) Read(p []byte) (int, error) {
	n, err := r.inner.Read(p)
	if n > 0 {
		r.read += int64(n)
		if r.read-r.last >= progressReportInterval || err == io.EOF {
			r.last = r.read
			if r.onRead != nil {
				r.onRead(InstallProgress{Phase: "downloading", Downloaded: r.read, Total: r.total})
			}
		}
	}
	return n, err
}
