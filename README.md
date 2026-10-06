# MNE Lab

MNE Lab is a desktop laboratory application for organizing, analyzing and
presenting LIGHTSCATTERING measurements (NanoBrook 90Plus exports). It is
built to run well on inexpensive computers: one small native executable, no
bundled browser engine, low memory use.

## What it does

- Up to five protected profiles per account, each with its own storage choice:
  **USB Drive + Cloud**, **USB Drive Only** or **Cloud Only**.
- Sync to the person's own cloud: Google Drive / Google One, iCloud or
  Microsoft OneDrive.
- **Portable USB Mode** (everything lives on the drive) and **Temporary Machine
  Mode** (nothing stays on the computer after Save, Clean & Exit).
- LIGHTSCATTERING library: import, graphs, stability cycles, statistics and
  presentation.
- Smart Export: figures (PNG, SVG, PDF, TIFF, JPEG, WebP), data (XLSX, CSV,
  TSV, TXT, JSON) and research packages (ZIP), with provenance.
- Phone access by QR code: viewer and presentation controller.
- Automatic updates, LockedBuild and the Stable build selector.
- Interface in English, Portuguese (Brazil) and Spanish, following the
  device language.

## Build

Requirements: Go 1.26 and Node.js 22.

```sh
cd web
npm ci
npm run build
cd ..
scripts/build.sh
```

`scripts/build.sh` writes the application and the Portable USB launcher for
Windows (x64, x86, ARM64) and Linux (x64, ARM64) into `out/`. Windows builds
carry the MNE Lab icon and version information and open without a console
window. macOS builds need cgo (tray icon), so they are built on a Mac with
`scripts/build.sh darwin/arm64 darwin/amd64`. `VERSION` and `CHANNEL` set the
version written into the executables.

Cloud provider credentials are not part of the repository. Release builds
receive them through `-ldflags -X`; see
[docs/TECHNICAL_CONTEXT.md](docs/TECHNICAL_CONTEXT.md).

## Checks

```sh
cd web && npm run typecheck && npm test && cd ..
gofmt -l . && go vet ./... && go test -race ./...
```

Browser tests (needs Chromium: `cd web && npx playwright-core install chromium`):

```sh
scripts/e2e.sh
```

They open every screen in English, Portuguese and Spanish, in both themes and
two window sizes, and the phone flow, and save screenshots to `out/e2e/`.

## Layout

| Path | Contents |
|---|---|
| `cmd/mnelab` | The application. |
| `cmd/mnelab-launcher` | Launcher for Portable USB installs. |
| `cmd/mnelab-release` | Release signing and manifest tool. |
| `internal/` | Core packages (storage, sync, providers, updates, science, export). |
| `web/` | Desktop and phone interface. |
| `web/tests/e2e` | Browser tests. |
| `scripts/` | Build, browser tests and repository audit. |
| `assets/brand` | MNE Lab icons and marks. |
| `testdata/` | Synthetic test files. |

## More

- [Technical context](docs/TECHNICAL_CONTEXT.md): architecture, decisions,
  status and next steps.
- [Third-party notices](THIRD_PARTY_NOTICES.md).

made by oaovito
