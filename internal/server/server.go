// Package server implements the dropserve HTTP file server.
// Author: Kurt Mitchell
// License: MIT
package server

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"
)

const defaultPort = 8000

// mimeTypes maps lowercase file extensions to their Content-Type values.
// These take precedence over Go's mime package and the OS MIME registry,
// which can differ between platforms (especially on Windows).
var mimeTypes = map[string]string{
	".html":  "text/html; charset=utf-8",
	".htm":   "text/html; charset=utf-8",
	".css":   "text/css; charset=utf-8",
	".js":    "application/javascript; charset=utf-8",
	".mjs":   "application/javascript; charset=utf-8",
	".json":  "application/json; charset=utf-8",
	".jsonc": "application/json; charset=utf-8",
	".map":   "application/json",
	".svg":   "image/svg+xml",
	".png":   "image/png",
	".jpg":   "image/jpeg",
	".jpeg":  "image/jpeg",
	".gif":   "image/gif",
	".ico":   "image/x-icon",
	".webp":  "image/webp",
	".woff":  "font/woff",
	".woff2": "font/woff2",
	".ttf":   "font/ttf",
	".otf":   "font/otf",
	".eot":   "application/vnd.ms-fontobject",
	".txt":   "text/plain; charset=utf-8",
	".md":    "text/plain; charset=utf-8",
	".xml":   "application/xml; charset=utf-8",
	".pdf":   "application/pdf",
	".zip":   "application/zip",
	".gz":    "application/gzip",
	".mp4":   "video/mp4",
	".webm":  "video/webm",
	".mp3":   "audio/mpeg",
	".wav":   "audio/wav",
	".yaml":  "text/yaml; charset=utf-8",
	".yml":   "text/yaml; charset=utf-8",
	".toml":  "text/plain; charset=utf-8",
}

// Config holds the fully resolved server configuration.
type Config struct {
	Port        int
	Root        string
	OpenBrowser bool
}

// tomlConfig mirrors the dropserve.toml schema.
type tomlConfig struct {
	Port        int    `toml:"port"`
	Root        string `toml:"root"`
	OpenBrowser bool   `toml:"open_browser"`
}

// LoadConfig builds a Config by layering: defaults → dropserve.toml → CLI flags.
// flagPort == 0 and flagDir == "" mean "not provided by caller".
func LoadConfig(flagPort int, flagDir string, flagOpen bool) (*Config, error) {
	cfg := &Config{
		Port:        defaultPort,
		Root:        ".",
		OpenBrowser: false,
	}

	const configFile = "dropserve.toml"
	if _, err := os.Stat(configFile); err == nil {
		var tc tomlConfig
		if _, err := toml.DecodeFile(configFile, &tc); err != nil {
			return nil, fmt.Errorf("malformed %s: %w", configFile, err)
		}
		if tc.Port != 0 {
			cfg.Port = tc.Port
		}
		if tc.Root != "" {
			cfg.Root = tc.Root
		}
		cfg.OpenBrowser = tc.OpenBrowser
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("cannot read %s: %w", configFile, err)
	}

	// CLI flags win over config file.
	if flagPort != 0 {
		cfg.Port = flagPort
	}
	if flagDir != "" {
		cfg.Root = flagDir
	}
	if flagOpen {
		cfg.OpenBrowser = true
	}

	absRoot, err := filepath.Abs(cfg.Root)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve directory %q: %w", cfg.Root, err)
	}
	cfg.Root = absRoot

	info, err := os.Stat(cfg.Root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("directory not found: %q", cfg.Root)
		}
		return nil, fmt.Errorf("cannot access %q: %w", cfg.Root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%q is not a directory", cfg.Root)
	}

	if cfg.Port < 1 || cfg.Port > 65535 {
		return nil, fmt.Errorf("port %d is out of range (1–65535)", cfg.Port)
	}

	return cfg, nil
}

// Run starts the HTTP server and blocks until SIGINT/SIGTERM.
func Run(cfg *Config) error {
	// Bind before printing the banner so port conflicts surface immediately.
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.Port))
	if err != nil {
		return friendlyNetError(err, cfg.Port)
	}

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: NewHandler(cfg.Root),
	}

	printBanner(cfg)

	if cfg.OpenBrowser {
		url := fmt.Sprintf("http://localhost:%d", cfg.Port)
		go func() {
			// Small delay so the server is ready before the browser hits it.
			time.Sleep(150 * time.Millisecond)
			if err := openBrowser(url); err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not open browser: %v\n", err)
			}
		}()
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	serverErr := make(chan error, 1)
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return err
	case <-quit:
		fmt.Println("\nShutting down — goodbye!")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(ctx)
	}
}

// NewHandler returns an http.Handler that serves static files from root.
// Exported so tests can construct a handler without running a full server.
func NewHandler(root string) http.Handler {
	return &fileHandler{root: filepath.Clean(root)}
}

// fileHandler serves static files, delegating to a directory listing when
// no index.html is present.
type fileHandler struct {
	root string
}

func (h *fileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Clean the URL path to remove any ".." or "." components.
	upath := path.Clean("/" + r.URL.Path)

	fsPath := filepath.Join(h.root, filepath.FromSlash(upath))

	// Belt-and-suspenders: reject any path that escapes the root after Join.
	// path.Clean already removes "..", but this guards against symlink tricks
	// and any future edge cases in filepath.Join behaviour.
	rootWithSep := filepath.Clean(h.root) + string(filepath.Separator)
	if fsPath != filepath.Clean(h.root) && !strings.HasPrefix(fsPath+string(filepath.Separator), rootWithSep) {
		http.Error(w, "403 Forbidden", http.StatusForbidden)
		return
	}

	info, err := os.Stat(fsPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "500 Internal Server Error", http.StatusInternalServerError)
		return
	}

	if info.IsDir() {
		// Redirect bare directory paths to their trailing-slash form so that
		// relative links inside index.html resolve correctly.
		if !strings.HasSuffix(r.URL.Path, "/") {
			http.Redirect(w, r, r.URL.Path+"/", http.StatusMovedPermanently)
			return
		}
		indexPath := filepath.Join(fsPath, "index.html")
		if _, err := os.Stat(indexPath); err == nil {
			serveFileWithMIME(w, r, indexPath)
			return
		}
		serveDirListing(w, r, fsPath, upath)
		return
	}

	serveFileWithMIME(w, r, fsPath)
}

// serveFileWithMIME sets Content-Type from our map (before http.ServeFile can
// override it) then delegates to http.ServeFile for ETags, Range, and
// Last-Modified support.
func serveFileWithMIME(w http.ResponseWriter, r *http.Request, fsPath string) {
	ext := strings.ToLower(filepath.Ext(fsPath))
	if ct, ok := mimeTypes[ext]; ok {
		// http.ServeFile only sets Content-Type when the header is absent,
		// so setting it here pins our value.
		w.Header().Set("Content-Type", ct)
	}
	http.ServeFile(w, r, fsPath)
}

// dirEntry is the data passed to the directory listing template.
type dirEntry struct {
	Name    string
	IsDir   bool
	Size    string
	ModTime string
}

var dirListTmpl = template.Must(template.New("dirlist").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Index of {{.Path}}</title>
  <style>
    *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
      font-size: 14px; color: #24292f; background: #fff;
      padding: 40px 24px; max-width: 900px; margin: 0 auto;
    }
    h1 { font-size: 16px; font-weight: 600; margin-bottom: 24px; color: #24292f; }
    table { width: 100%; border-collapse: collapse; }
    th {
      text-align: left; padding: 6px 12px;
      border-bottom: 2px solid #d0d7de;
      color: #57606a; font-size: 11px; font-weight: 600;
      text-transform: uppercase; letter-spacing: .06em;
    }
    td { padding: 5px 12px; border-bottom: 1px solid #f0f0f0; }
    tr:last-child td { border-bottom: none; }
    tr:hover td { background: #f6f8fa; }
    a { color: #0969da; text-decoration: none; }
    a:hover { text-decoration: underline; }
    .dir { font-weight: 600; }
    .right { text-align: right; color: #57606a; }
    footer { margin-top: 32px; color: #8c959f; font-size: 11px; }
  </style>
</head>
<body>
  <h1>Index of {{.Path}}</h1>
  <table>
    <thead>
      <tr>
        <th>Name</th>
        <th class="right">Modified</th>
        <th class="right">Size</th>
      </tr>
    </thead>
    <tbody>
      {{if ne .Path "/"}}
      <tr>
        <td><a href="../">../</a></td>
        <td class="right"></td>
        <td class="right"></td>
      </tr>
      {{end}}
      {{range .Entries}}
      <tr>
        <td>
          {{- if .IsDir -}}
          <a class="dir" href="{{.Name}}/">{{.Name}}/</a>
          {{- else -}}
          <a href="{{.Name}}">{{.Name}}</a>
          {{- end -}}
        </td>
        <td class="right">{{.ModTime}}</td>
        <td class="right">{{if .IsDir}}&mdash;{{else}}{{.Size}}{{end}}</td>
      </tr>
      {{end}}
    </tbody>
  </table>
  <footer>dropserve</footer>
</body>
</html>
`))

func serveDirListing(w http.ResponseWriter, r *http.Request, fsPath, urlPath string) {
	entries, err := os.ReadDir(fsPath)
	if err != nil {
		http.Error(w, "500 Internal Server Error", http.StatusInternalServerError)
		return
	}

	var dirs, files []dirEntry
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		de := dirEntry{
			Name:    e.Name(),
			IsDir:   e.IsDir(),
			ModTime: info.ModTime().Format("2006-01-02 15:04"),
		}
		if e.IsDir() {
			dirs = append(dirs, de)
		} else {
			de.Size = formatSize(info.Size())
			files = append(files, de)
		}
	}
	// Directories first, each group sorted alphabetically.
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name < dirs[j].Name })
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })

	data := struct {
		Path    string
		Entries []dirEntry
	}{
		Path:    urlPath,
		Entries: append(dirs, files...),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = dirListTmpl.Execute(w, data)
}

func printBanner(cfg *Config) {
	url := fmt.Sprintf("http://localhost:%d", cfg.Port)
	fmt.Println()
	fmt.Println("  dropserve")
	fmt.Println()
	fmt.Printf("  URL   → %s\n", url)
	fmt.Printf("  Port  → %d\n", cfg.Port)
	fmt.Printf("  Root  → %s\n", cfg.Root)
	fmt.Println()
	fmt.Println("  Press Ctrl+C to stop.")
	fmt.Println()
}

func openBrowser(url string) error {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default: // linux, bsd, etc.
		cmd = "xdg-open"
		args = []string{url}
	}
	return exec.Command(cmd, args...).Start()
}

// friendlyNetError translates raw listen errors into actionable messages.
func friendlyNetError(err error, port int) error {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		msg := opErr.Err.Error()
		switch {
		case strings.Contains(msg, "address already in use"):
			return fmt.Errorf("port %d is already in use — try a different port with -p", port)
		case strings.Contains(msg, "permission denied"):
			return fmt.Errorf("permission denied on port %d — ports below 1024 require elevated privileges", port)
		}
	}
	return err
}

func formatSize(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	const unit = 1024
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
