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
- Windows release builds use LLVM-MinGW and the SpiderMonkey `windows-amd64`
  target.
- Linux release builds use GCC on Ubuntu 24.04 x86_64 and the SpiderMonkey
  `linux-amd64-gnu` target. The resulting binary requires glibc 2.39 or newer.

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

For Linux, install a C/C++ toolchain and point cgo at the pinned
SpiderMonkey `linux-amd64-gnu` headers and libraries:

```bash
export CGO_CXXFLAGS="-I/path/to/spidermonkey/include"
export CGO_LDFLAGS="-L/path/to/spidermonkey/lib"
CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o dist/aistudio-cli ./cmd/aistudio-cli
```

## Serve

```bash
aistudio-cli serve --auth-dir auth --listen 127.0.0.1:2048
```

Optional flags:

- `--api-key`: require `Authorization: Bearer <key>` on public API requests.
- `--proxy`: global HTTP/SOCKS5 proxy.
- `--waa-backend`: WAA backend, either `spidermonkey` (default) or `camoufox`.
  The default can also be set with the `WAA_BACKEND` environment variable; the
  flag takes precedence.
- `--camoufox-path`: use a specific Camoufox executable. When neither
  `--waa-backend` nor `WAA_BACKEND` selects a backend, setting this path selects
  the `camoufox` backend.
- `--init-timeout`, `--startup-timeout`, `--request-timeout`: timeout controls.
- `--temporary-chat`, `--prewarm`, `--verbose`: WAA and logging options.

The `spidermonkey` backend is the default on all platforms and does not require
a Camoufox download, but it still needs the cgo SpiderMonkey libraries at build
time. The `camoufox` backend is opt-in; on headless Linux servers it also needs
`Xvfb`, Mesa/GLX, and the usual Firefox runtime libraries.

The server exposes `/v1/`, `/v1beta/`, `/upload/`, and `/health`. The upstream
admin/control-plane routes are not part of this CLI and have been removed.

## Releases

Tags matching `v*` build the CLI on GitHub Actions and publish:

- `aistudio-cli.exe`
- `aistudio-cli-<tag>-windows-amd64.zip`
- `aistudio-cli-<tag>-linux-amd64.tar.gz`

## License

MIT. The original copyright notice is preserved in [LICENSE](LICENSE).
