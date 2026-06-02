package server_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kurtmitchell/dropserve/internal/server"
)

// chdir changes to dir and returns a function that restores the original
// working directory. Intended for use with defer: defer chdir(t, dir)()
func chdir(t *testing.T, dir string) func() {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %q: %v", dir, err)
	}
	return func() {
		if err := os.Chdir(orig); err != nil {
			t.Logf("warning: could not restore working directory: %v", err)
		}
	}
}

// --- Config tests ---

func TestLoadConfigDefaults(t *testing.T) {
	tmp := t.TempDir()
	defer chdir(t, tmp)()

	cfg, err := server.LoadConfig(0, "", false)
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}

	if cfg.Port != 8000 {
		t.Errorf("default port: got %d, want 8000", cfg.Port)
	}
	if cfg.OpenBrowser {
		t.Error("default OpenBrowser: got true, want false")
	}

	absTemp, _ := filepath.Abs(tmp)
	if cfg.Root != absTemp {
		t.Errorf("default root: got %q, want %q", cfg.Root, absTemp)
	}
}

func TestLoadConfigFromTOML(t *testing.T) {
	tmp := t.TempDir()
	defer chdir(t, tmp)()

	serveDir := filepath.Join(tmp, "public")
	if err := os.Mkdir(serveDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Use %q so the path is properly quoted in the TOML string value.
	content := fmt.Sprintf("port = 9999\nroot = %q\nopen_browser = true\n", serveDir)
	if err := os.WriteFile("dropserve.toml", []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := server.LoadConfig(0, "", false)
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	if cfg.Port != 9999 {
		t.Errorf("toml port: got %d, want 9999", cfg.Port)
	}
	if !cfg.OpenBrowser {
		t.Error("toml open_browser: got false, want true")
	}

	absServeDir, _ := filepath.Abs(serveDir)
	if cfg.Root != absServeDir {
		t.Errorf("toml root: got %q, want %q", cfg.Root, absServeDir)
	}
}

func TestFlagOverridesConfig(t *testing.T) {
	tmp := t.TempDir()
	defer chdir(t, tmp)()

	if err := os.WriteFile("dropserve.toml", []byte("port = 9999\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := server.LoadConfig(7777, "", false)
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	if cfg.Port != 7777 {
		t.Errorf("flag should override toml: got port %d, want 7777", cfg.Port)
	}
}

func TestMalformedTOMLReturnsError(t *testing.T) {
	tmp := t.TempDir()
	defer chdir(t, tmp)()

	if err := os.WriteFile("dropserve.toml", []byte("this is not valid toml ==="), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := server.LoadConfig(0, "", false)
	if err == nil {
		t.Fatal("expected error for malformed TOML, got nil")
	}
	if !strings.Contains(err.Error(), "dropserve.toml") {
		t.Errorf("error should mention the config file name; got: %v", err)
	}
}

func TestInvalidPortReturnsError(t *testing.T) {
	tmp := t.TempDir()
	defer chdir(t, tmp)()

	_, err := server.LoadConfig(99999, "", false)
	if err == nil {
		t.Error("expected error for out-of-range port, got nil")
	}
}

func TestMissingDirReturnsError(t *testing.T) {
	tmp := t.TempDir()
	defer chdir(t, tmp)()

	_, err := server.LoadConfig(0, filepath.Join(tmp, "nonexistent"), false)
	if err == nil {
		t.Error("expected error for missing directory, got nil")
	}
}

// --- HTTP handler tests ---

func TestServeIndexHTML(t *testing.T) {
	tmp := t.TempDir()
	body := "<html><body>hello dropserve</body></html>"
	if err := os.WriteFile(filepath.Join(tmp, "index.html"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(server.NewHandler(tmp))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /: status %d, want 200", resp.StatusCode)
	}

	got, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(got), "hello dropserve") {
		t.Errorf("body missing expected content; got: %s", got)
	}
}

func TestServeDirListingWhenNoIndex(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "style.css"), []byte("body{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(tmp, "assets"), 0755); err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(server.NewHandler(tmp))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /: status %d, want 200", resp.StatusCode)
	}

	got, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(got), "style.css") {
		t.Errorf("directory listing missing style.css; got: %s", got)
	}
	if !strings.Contains(string(got), "assets") {
		t.Errorf("directory listing missing assets/; got: %s", got)
	}
}

func TestServeFileContentType(t *testing.T) {
	tests := []struct {
		file    string
		content string
		wantCT  string
	}{
		{"app.js", "console.log('hi')", "javascript"},
		{"style.css", "body{}", "css"},
		{"data.json", "{}", "json"},
		{"icon.svg", "<svg/>", "svg"},
	}

	for _, tc := range tests {
		t.Run(tc.file, func(t *testing.T) {
			tmp := t.TempDir()
			if err := os.WriteFile(filepath.Join(tmp, tc.file), []byte(tc.content), 0644); err != nil {
				t.Fatal(err)
			}

			ts := httptest.NewServer(server.NewHandler(tmp))
			defer ts.Close()

			resp, err := http.Get(ts.URL + "/" + tc.file)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()

			ct := resp.Header.Get("Content-Type")
			if !strings.Contains(ct, tc.wantCT) {
				t.Errorf("Content-Type for %s: got %q, want substring %q", tc.file, ct, tc.wantCT)
			}
		})
	}
}

func TestNotFoundReturns404(t *testing.T) {
	tmp := t.TempDir()
	ts := httptest.NewServer(server.NewHandler(tmp))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/does-not-exist.html")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("missing file: status %d, want 404", resp.StatusCode)
	}
}

func TestDirRedirectsToTrailingSlash(t *testing.T) {
	tmp := t.TempDir()
	sub := filepath.Join(tmp, "sub")
	if err := os.Mkdir(sub, 0755); err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(server.NewHandler(tmp))
	defer ts.Close()

	// Disable automatic redirect following so we can inspect the 301.
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	resp, err := client.Get(ts.URL + "/sub")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusMovedPermanently {
		t.Errorf("bare dir: status %d, want 301", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/sub/" {
		t.Errorf("redirect Location: got %q, want /sub/", loc)
	}
}
