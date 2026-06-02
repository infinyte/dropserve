# dropserve

A zero-config, single-binary static file server for local development.  
Drop it in a folder, run it, done.

```
  dropserve

  URL   → http://localhost:8000
  Port  → 8000
  Root  → /home/you/project/dist

  Press Ctrl+C to stop.
```

---

## Install

### Pre-built binary

Download the binary for your platform from the [releases page](https://github.com/kurtmitchell/dropserve/releases), make it executable, and place it on your `PATH`.

```bash
# macOS (Apple Silicon)
curl -L https://github.com/kurtmitchell/dropserve/releases/latest/download/dropserve-darwin-arm64 \
  -o /usr/local/bin/dropserve && chmod +x /usr/local/bin/dropserve

# Linux (amd64)
curl -L https://github.com/kurtmitchell/dropserve/releases/latest/download/dropserve-linux-amd64 \
  -o /usr/local/bin/dropserve && chmod +x /usr/local/bin/dropserve
```

### Build from source

Requires Go 1.21+.

```bash
git clone https://github.com/kurtmitchell/dropserve
cd dropserve
make build          # produces ./dropserve
# or
go install github.com/kurtmitchell/dropserve/cmd/dropserve@latest
```

---

## Usage

```
dropserve [flags]

Flags:
  -p, -port   int     Port to serve on (default: 8000)
  -d, -dir    string  Directory to serve (default: current directory)
  -o, -open           Open the default browser on start
  -v, -version        Print version and exit
  -h, -help           Print usage and exit
```

### Examples

```bash
# Serve the current directory on :8000
dropserve

# Serve a specific directory
dropserve -d ./dist

# Choose a different port
dropserve -p 3000

# Serve ./docs on :9000 and open in browser
dropserve -d ./docs -p 9000 -o

# Serve a Swagger UI build
cd swagger-ui/dist && dropserve
```

---

## Config file

Create `dropserve.toml` in the directory where you run `dropserve`.  
All keys are optional. CLI flags override config file values.

```toml
# dropserve.toml
port         = 3000
root         = "./dist"
open_browser = true
```

| Key           | Type   | Default           | Description                        |
|---------------|--------|-------------------|------------------------------------|
| `port`        | int    | `8000`            | TCP port to listen on              |
| `root`        | string | current directory | Directory to serve                 |
| `open_browser`| bool   | `false`           | Open browser on start              |

---

## Features

- **Zero config** — `dropserve` just works
- **Directory listing** — clean HTML listing when no `index.html` is present
- **`index.html` auto-serve** — any directory with an `index.html` serves it directly
- **Correct MIME types** — covers HTML, CSS, JS, JSON, SVG, PNG/JPG/WEBP, WOFF/WOFF2, fonts, video, and more — consistent across all platforms
- **Graceful shutdown** — Ctrl+C drains in-flight requests cleanly
- **Helpful errors** — port conflicts, missing directories, and bad configs produce readable messages, not Go stack traces

---

## Cross-compilation

Build all platforms at once (requires Docker/Go cross-compile support):

```bash
make dist
# or
./build.sh dist
```

Output in `dist/`:

| File                              | Platform              |
|-----------------------------------|-----------------------|
| `dropserve-linux-amd64`           | Linux x86-64          |
| `dropserve-linux-arm64`           | Linux ARM64           |
| `dropserve-darwin-amd64`          | macOS Intel           |
| `dropserve-darwin-arm64`          | macOS Apple Silicon   |
| `dropserve-windows-amd64.exe`     | Windows x86-64        |

Manual cross-compile for a single target:

```bash
GOOS=windows GOARCH=amd64 go build \
  -ldflags "-s -w -X 'main.version=0.1.0'" \
  -o dist/dropserve-windows-amd64.exe \
  ./cmd/dropserve
```

Common `GOOS`/`GOARCH` pairs:

| `GOOS`    | `GOARCH` | Target                     |
|-----------|----------|----------------------------|
| `linux`   | `amd64`  | Linux 64-bit (most servers)|
| `linux`   | `arm64`  | Raspberry Pi 4, AWS Graviton|
| `darwin`  | `amd64`  | macOS Intel                |
| `darwin`  | `arm64`  | macOS Apple Silicon (M1+)  |
| `windows` | `amd64`  | Windows 64-bit             |

---

## Development

```bash
# Run tests
make test

# Build for current platform
make build

# Clean build artifacts
make clean
```

---

## License

MIT — see [LICENSE](LICENSE).  
Author: Kurt Mitchell
