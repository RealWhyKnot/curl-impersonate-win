# curl-impersonate-win

This repository is archived.

Use [lexiforest/curl-impersonate](https://github.com/lexiforest/curl-impersonate)
for maintained browser impersonation builds. Its Windows release packages include
`curl-impersonate.exe`, browser wrapper scripts, and `libcurl-impersonate.dll`.

## Historical Scope

`curl-impersonate-win` was a small Windows-only subprocess wrapper around
`bogdanfinn/tls-client`. It provided a single static executable with a narrow
curl-like interface:

```text
curl-impersonate-win [--impersonate <profile>] [-s] [-i] [-X METHOD]
                     [-H "Key: Value"] [--max-time <seconds>] <url>
```

The repository is kept for historical reference. New projects should use
`lexiforest/curl-impersonate` instead.
