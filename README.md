# aistudio-cli

Standalone command-line client for Google AI Studio Playground private APIs.
This repository contains the CLI and the minimum internal packages required to
build it.

The code is derived from
[Mag1cFall/AIStudio2API](https://github.com/Mag1cFall/AIStudio2API) and is
distributed under the same MIT license. See [LICENSE](LICENSE).

## Features

- `account list` / `account import`: inspect and import account cookies.
- `models`: query the available AI Studio model catalog.
- `generate`: call the Playground `GenerateContent` API.
- `serve`: expose an OpenAI-compatible HTTP API.

## Requirements

- Go 1.27 or newer.
- A C compiler and the SpiderMonkey static libraries for cgo builds.
- Windows release builds use LLVM-MinGW and the SpiderMonkey
  `windows-amd64` target.

The GitHub workflow downloads the same pinned SpiderMonkey build and
LLVM-MinGW version used by the upstream project, so `.deps/` is not committed.

## Build

```bash
go build -o bin/aistudio-cli ./cmd/aistudio-cli
```

For Windows cross-compilation:

```bash
CGO_ENABLED=1 GOOS=windows GOARCH=amd64 \
CC=x86_64-w64-mingw32-clang \
CXX=x86_64-w64-mingw32-clang++ \
go build -trimpath -ldflags="-s -w" -o dist/aistudio-cli.exe ./cmd/aistudio-cli
```

## Serve

```bash
aistudio-cli serve --auth-dir auth --listen 127.0.0.1:2048
```

Optional flags:

- `--api-key`: require `Authorization: Bearer <key>` on public API requests.
- `--proxy`: global HTTP/SOCKS5 proxy.
- `--waa-backend`: WAA backend, either `camoufox` (default) or `spidermonkey`.
  The default can also be set with the `WAA_BACKEND` environment variable; the
  flag takes precedence.
- `--camoufox-path`: use a specific Camoufox executable (camoufox backend only).
- `--init-timeout`, `--startup-timeout`, `--request-timeout`: timeout controls.
- `--temporary-chat`, `--prewarm`, `--verbose`: WAA and logging options.

The `spidermonkey` backend does not require a Camoufox download, but it still
needs the cgo SpiderMonkey libraries at build time.

The server exposes `/v1/`, `/v1beta/`, `/upload/`, and `/health`. The upstream
admin/control-plane routes are not part of this CLI and have been removed.

## Releases

Tags matching `v*` build `aistudio-cli.exe` on GitHub Actions and publish:

- `aistudio-cli.exe`
- `aistudio-cli-<tag>-windows-amd64.zip`

## License

MIT. The original copyright notice is preserved in [LICENSE](LICENSE).
