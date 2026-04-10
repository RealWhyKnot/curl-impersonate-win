package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	fhttp "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

type config struct {
	impersonate    string
	silent         bool
	includeHeaders bool
	method         string
	headers        []string
	maxTime        int
	url            string
}

// profileMap maps curl-impersonate-style profile names (lowercase, no separators)
// to bogdanfinn/tls-client profile constants.
var profileMap = map[string]profiles.ClientProfile{
	// Chrome — bare version aliases (curl-impersonate convention)
	"chrome103": profiles.Chrome_103,
	"chrome104": profiles.Chrome_104,
	"chrome105": profiles.Chrome_105,
	"chrome106": profiles.Chrome_106,
	"chrome107": profiles.Chrome_107,
	"chrome108": profiles.Chrome_108,
	"chrome109": profiles.Chrome_109,
	"chrome110": profiles.Chrome_110,
	"chrome111": profiles.Chrome_111,
	"chrome112": profiles.Chrome_112,
	// Chrome 116+ uses PSK (session resumption) by default, matching real browser behaviour
	"chrome116": profiles.Chrome_116_PSK,
	"chrome117": profiles.Chrome_117,
	"chrome120": profiles.Chrome_120,
	"chrome124": profiles.Chrome_124,
	"chrome130": profiles.Chrome_130_PSK,
	"chrome131": profiles.Chrome_131,
	"chrome133": profiles.Chrome_133, // default profile
	"chrome144": profiles.Chrome_144,
	"chrome146": profiles.Chrome_146,

	// Chrome — explicit PSK / PQ variants (for callers that want them by name)
	"chrome116psk":   profiles.Chrome_116_PSK,
	"chrome116pskpq": profiles.Chrome_116_PSK_PQ,
	"chrome130psk":   profiles.Chrome_130_PSK,
	"chrome131psk":   profiles.Chrome_131_PSK,
	"chrome133psk":   profiles.Chrome_133_PSK,
	"chrome144psk":   profiles.Chrome_144_PSK,
	"chrome146psk":   profiles.Chrome_146_PSK,

	// Firefox
	"firefox102": profiles.Firefox_102,
	"firefox104": profiles.Firefox_104,
	"firefox105": profiles.Firefox_105,
	"firefox106": profiles.Firefox_106,
	"firefox108": profiles.Firefox_108,
	"firefox110": profiles.Firefox_110,
	"firefox117": profiles.Firefox_117,
	"firefox120": profiles.Firefox_120,
	"firefox123": profiles.Firefox_123,
	"firefox132": profiles.Firefox_132,
	"firefox133": profiles.Firefox_133,
	"firefox135": profiles.Firefox_135,
	"firefox147": profiles.Firefox_147,

	// Safari
	"safari1561": profiles.Safari_15_6_1,
	"safari160":  profiles.Safari_16_0,
}

// resolveProfile maps a user-supplied name (any capitalisation / separator style)
// to a tls-client ClientProfile. Falls back to the library's own MappedTLSClients.
func resolveProfile(name string) (profiles.ClientProfile, error) {
	// Normalise: lowercase, drop -, _, .
	key := strings.ToLower(strings.NewReplacer("-", "", "_", "", ".", "").Replace(name))
	if p, ok := profileMap[key]; ok {
		return p, nil
	}
	// Library fallback (uses its own key format, e.g. "Chrome_116_PSK")
	if p, ok := profiles.MappedTLSClients[name]; ok {
		return p, nil
	}
	return profiles.ClientProfile{}, fmt.Errorf(
		"unknown profile %q — supported profiles: chrome116, chrome120, chrome133, firefox117, etc. (see README)",
		name,
	)
}

// parseArgs walks os.Args[1:] and populates a config.
// It is hand-rolled (not flag.Parse) to support:
//   - Repeated -H flags
//   - Positional URL as last bare argument
//   - Double-dash long flags alongside single-dash short flags
//   - Unknown flags silently ignored (curl-compatible)
func parseArgs(args []string) (config, error) {
	cfg := config{impersonate: "chrome133", method: "GET"}

	for i := 0; i < len(args); i++ {
		arg := args[i]

		// consume the next token or return an error
		next := func() (string, error) {
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s requires an argument", arg)
			}
			i++
			return args[i], nil
		}

		switch arg {
		case "--impersonate":
			v, err := next()
			if err != nil {
				return cfg, err
			}
			cfg.impersonate = v

		case "-s", "--silent":
			cfg.silent = true

		case "-i", "--include":
			cfg.includeHeaders = true

		case "-X", "--request":
			v, err := next()
			if err != nil {
				return cfg, err
			}
			cfg.method = v

		case "-H", "--header":
			v, err := next()
			if err != nil {
				return cfg, err
			}
			cfg.headers = append(cfg.headers, v)

		case "--max-time":
			v, err := next()
			if err != nil {
				return cfg, err
			}
			cfg.maxTime, _ = strconv.Atoi(v)

		default:
			switch {
			case strings.HasPrefix(arg, "-H") && len(arg) > 2:
				// -HValue (no space between flag and value)
				cfg.headers = append(cfg.headers, arg[2:])
			case !strings.HasPrefix(arg, "-"):
				cfg.url = arg
			// Unknown flags are silently accepted (mirrors curl behaviour)
			}
		}
	}

	if cfg.url == "" {
		return cfg, fmt.Errorf("no URL specified\nUsage: curl-impersonate-win [--impersonate <profile>] [-s] [-i] [-X METHOD] [-H \"Key: Value\"] <url>")
	}
	return cfg, nil
}

// writeStatusLine writes the HTTP status line (e.g. "HTTP/1.1 200 OK\r\n" or
// "HTTP/2 200\r\n") to w in curl -i format.
func writeStatusLine(w *bytes.Buffer, resp *fhttp.Response) {
	proto := resp.Proto
	if proto == "HTTP/2.0" {
		proto = "HTTP/2" // normalise Go's internal representation
	}
	if strings.HasPrefix(proto, "HTTP/2") {
		// HTTP/2 status lines carry no reason phrase (RFC 9113 §8.3.1)
		fmt.Fprintf(w, "%s %d\r\n", proto, resp.StatusCode)
	} else {
		// resp.Status is "200 OK"; take everything after the first space
		reason := resp.Status
		if sp := strings.IndexByte(resp.Status, ' '); sp >= 0 {
			reason = resp.Status[sp+1:]
		}
		fmt.Fprintf(w, "%s %d %s\r\n", proto, resp.StatusCode, reason)
	}
}

// exitCode maps a tls-client error to a curl-compatible exit code.
func exitCode(err error) int {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "timeout") || strings.Contains(msg, "context deadline exceeded"):
		return 28
	case strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "invalid URL") ||
		strings.Contains(msg, "unsupported protocol"):
		return 6
	case strings.Contains(msg, "connection refused"):
		return 7
	case strings.Contains(msg, "tls") ||
		strings.Contains(msg, "TLS") ||
		strings.Contains(msg, "certificate"):
		return 35
	default:
		return 1
	}
}

func run() int {
	cfg, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "curl-impersonate-win: %v\n", err)
		return 2
	}

	profile, err := resolveProfile(cfg.impersonate)
	if err != nil {
		fmt.Fprintf(os.Stderr, "curl-impersonate-win: %v\n", err)
		return 2
	}

	opts := []tls_client.HttpClientOption{
		tls_client.WithClientProfile(profile),
		tls_client.WithTimeoutSeconds(cfg.maxTime), // 0 = no timeout (critical for large streams)
	}
	client, err := tls_client.NewHttpClient(tls_client.NewNoopLogger(), opts...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "curl-impersonate-win: failed to create HTTP client: %v\n", err)
		return 1
	}

	req, err := fhttp.NewRequest(cfg.method, cfg.url, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "curl-impersonate-win: invalid request: %v\n", err)
		return 6
	}

	// Build request headers, preserving the order supplied by -H flags.
	req.Header = fhttp.Header{}
	var headerOrder []string
	for _, h := range cfg.headers {
		idx := strings.IndexByte(h, ':')
		if idx < 0 {
			fmt.Fprintf(os.Stderr, "curl-impersonate-win: warning: skipping malformed header (no colon): %q\n", h)
			continue
		}
		k := strings.TrimSpace(h[:idx])
		v := strings.TrimSpace(h[idx+1:])
		req.Header[k] = append(req.Header[k], v)
		headerOrder = append(headerOrder, k)
	}
	if len(headerOrder) > 0 {
		req.Header[fhttp.HeaderOrderKey] = headerOrder
	}

	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "curl-impersonate-win: %v\n", err)
		return exitCode(err)
	}
	defer resp.Body.Close()

	// Write headers to stdout before streaming the body.
	// All header bytes are assembled in a buffer first so the single Write
	// call is atomic — the C# consumer must never see a partial header block
	// interleaved with body bytes.
	if cfg.includeHeaders {
		var hdr bytes.Buffer
		writeStatusLine(&hdr, resp)
		for k, vals := range resp.Header {
			// Skip internal pseudo-headers (e.g. fhttp's HeaderOrderKey,
			// HTTP/2 pseudo-headers like :status)
			if k == fhttp.HeaderOrderKey || strings.HasPrefix(k, ":") {
				continue
			}
			for _, v := range vals {
				fmt.Fprintf(&hdr, "%s: %s\r\n", k, strings.TrimSpace(v))
			}
		}
		hdr.WriteString("\r\n") // blank line separating headers from body
		if _, err := os.Stdout.Write(hdr.Bytes()); err != nil {
			fmt.Fprintf(os.Stderr, "curl-impersonate-win: stdout write error: %v\n", err)
			return 1
		}
	}

	// Stream body directly to stdout — no intermediate buffering.
	// io.Copy uses a 32 KiB internal buffer, safe for multi-GB transfers.
	if _, err := io.Copy(os.Stdout, resp.Body); err != nil {
		fmt.Fprintf(os.Stderr, "curl-impersonate-win: body streaming error: %v\n", err)
		return 1
	}

	return 0
}

func main() {
	os.Exit(run())
}
