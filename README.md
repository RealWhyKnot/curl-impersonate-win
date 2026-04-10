# curl-impersonate-win

A single Windows binary that is a drop-in replacement for the
[curl-impersonate](https://github.com/lwthiker/curl-impersonate) CLI interface on Windows.

It uses [bogdanfinn/tls-client](https://github.com/bogdanfinn/tls-client) to forge realistic
TLS fingerprints (JA3/JA4) that match real browsers, bypassing TLS-based bot detection.

---

## Installation

Download the latest `curl-impersonate-win.exe` from the
[Releases](../../releases/latest) page and place it anywhere on your `PATH`.

No runtime dependencies — it is a fully static binary (CGO disabled).

---

## Usage

```
curl-impersonate-win [--impersonate <profile>] [-s] [-i] [-X METHOD]
                     [-H "Key: Value"] [--max-time <seconds>] <url>
```

| Flag | Description |
|------|-------------|
| `--impersonate <profile>` | TLS fingerprint profile to use (default: `chrome133`) |
| `-s` / `--silent` | Suppress progress output (no-op; included for curl compatibility) |
| `-i` / `--include` | Include HTTP response headers in stdout before the body |
| `-X METHOD` / `--request METHOD` | HTTP method — GET, HEAD, POST, etc. (default: `GET`) |
| `-H "Key: Value"` / `--header "Key: Value"` | Add a request header (can be repeated) |
| `--max-time <seconds>` | Timeout in seconds (default: 0 = unlimited) |
| `<url>` | Target URL (last positional argument) |

### Examples

```bat
:: Simple GET with Chrome 133 fingerprint
curl-impersonate-win --impersonate chrome133 -s https://example.com

:: Include response headers (curl -i style)
curl-impersonate-win --impersonate chrome120 -i https://httpbin.org/get

:: Custom headers + explicit method
curl-impersonate-win --impersonate firefox117 -X GET ^
  -H "Accept: application/json" ^
  -H "Referer: https://example.com" ^
  https://httpbin.org/headers

:: Verify TLS fingerprint
curl-impersonate-win --impersonate chrome133 -s https://tls.peet.ws/api/all
```

---

## Supported `--impersonate` profiles

Profile names are case-insensitive and separators (`-`, `_`, `.`) are ignored,
so `chrome-116`, `Chrome_116`, and `chrome116` are all equivalent.

### Chrome

| Profile | Notes |
|---------|-------|
| `chrome103` – `chrome112` | Chrome 103–112 |
| `chrome116` | Chrome 116 with PSK (session resumption) |
| `chrome117` | Chrome 117 |
| `chrome120` | Chrome 120 |
| `chrome124` | Chrome 124 |
| `chrome130` | Chrome 130 with PSK |
| `chrome131` | Chrome 131 |
| `chrome133` | Chrome 133 — **default** |
| `chrome144` | Chrome 144 |
| `chrome146` | Chrome 146 |
| `chrome116psk`, `chrome130psk`, … | Explicit PSK variants |
| `chrome116pskpq` | Chrome 116 with post-quantum key exchange |

### Firefox

| Profile |
|---------|
| `firefox102` – `firefox110` |
| `firefox117`, `firefox120`, `firefox123` |
| `firefox132`, `firefox133`, `firefox135`, `firefox147` |

### Safari

| Profile |
|---------|
| `safari1561` (Safari 15.6.1) |
| `safari160` (Safari 16.0) |

Any profile name supported by
[bogdanfinn/tls-client](https://github.com/bogdanfinn/tls-client/tree/master/profiles)
can also be passed using the library's native key format (e.g. `Chrome_116_PSK`).

---

## Output format (`-i`)

When `-i` is passed the output is byte-for-byte compatible with `curl -i`:

```
HTTP/1.1 200 OK\r\n
Content-Type: application/json\r\n
Content-Length: 256\r\n
\r\n
<raw body bytes>
```

For HTTP/2 responses the status line omits the reason phrase (per RFC 9113):

```
HTTP/2 200\r\n
content-type: application/json\r\n
\r\n
<raw body bytes>
```

The body is streamed directly to stdout with no intermediate buffering, making it
suitable for large (multi-GB) transfers.

---

## WKVRCProxy integration

`curl-impersonate-win` is designed to be called as a subprocess by C# applications
that need browser-like TLS fingerprints. The canonical consumer is **WKVRCProxy**.

### Spawning the process (C#)

```csharp
var psi = new ProcessStartInfo
{
    FileName  = "curl-impersonate-win.exe",
    Arguments = $"--impersonate chrome133 -i -s {url}",

    UseShellExecute        = false,
    RedirectStandardOutput = true,
    RedirectStandardError  = true,

    // IMPORTANT: set to null so stdout is treated as raw bytes,
    // not decoded as text. Without this, the CLR may corrupt binary body data.
    StandardOutputEncoding = null,
};

using var proc = Process.Start(psi)!;
```

### Parsing the response (C#)

```csharp
using var stdout = proc.StandardOutput.BaseStream;

// 1. Read until the blank line (\r\n\r\n) that terminates the headers.
var headerBytes = ReadUntilDoubleCrlf(stdout);  // your helper
var headerText  = Encoding.ASCII.GetString(headerBytes);

// 2. Split into lines to get status + individual headers.
var lines       = headerText.Split("\r\n", StringSplitOptions.RemoveEmptyEntries);
var statusLine  = lines[0];   // e.g. "HTTP/2 200" or "HTTP/1.1 200 OK"

// 3. Everything remaining in stdout is the body — stream it directly.
await stdout.CopyToAsync(responseBodyStream);

await proc.WaitForExitAsync();
```

**Key points:**
- Set `StandardOutputEncoding = null` — the body may be binary (video, audio, etc.).
- Do **not** read all of stdout into memory first; pipe it straight to your output
  stream to handle multi-GB payloads.
- Read stderr separately (or discard it) to capture error messages without
  blocking the stdout reader.

---

## Building from source

Prerequisites: [Go 1.22+](https://go.dev/dl/)

```bash
# Cross-compile for Windows from Linux/macOS/Windows
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
  go build -ldflags="-s -w" -o curl-impersonate-win.exe .
```

Or natively on Windows (PowerShell):

```powershell
$env:GOOS="windows"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
go build -ldflags="-s -w" -o curl-impersonate-win.exe .
```

---

## Automated releases

The [build workflow](.github/workflows/build.yml) runs on:

- Every push to `main`
- Every Monday at 06:00 UTC (scheduled)

For scheduled runs it checks whether
[bogdanfinn/tls-client](https://github.com/bogdanfinn/tls-client/releases) has
released a new version since the last build, and skips the rebuild if nothing has changed.

Release tags follow the format `vYYYY.MM.DD` (e.g. `v2026.04.10`).

---

## License

MIT
