# Release pipeline design

## Intent

Ship whoosh2connect as tagged GitHub releases built automatically for Linux,
Windows and macOS, with a Scoop manifest for Windows users. Follow the build
system of `thomaschampagne/zwiftboard`: a `justfile` as the single source of
build recipes, CI that runs the same recipes, and semantic-release driving
versions from conventional commits.

## Goals

- Every push to `main` that carries a releasable commit produces a semver tag
  and a GitHub Release with binaries for three platforms.
- The version a user runs is the version of the release: the TUI header and
  `--version` both show it.
- CI runs the same checks a developer runs locally (`just ci`).
- Windows users can `scoop install` from this repository, which is its own
  bucket. The manifest is regenerated on each release.

## Non-goals

- Submitting to ScoopInstaller/Extras. It needs a PAT and a human-reviewed PR,
  so it is a follow-up once a license and PAT exist.
- Code signing, notarization, installers, Homebrew, Docker.
- Changing the application behaviour.

## Constraints

- Go 1.27.2 (from `go.mod`). CGO disabled, so builds are static and portable.
- Platforms: `linux/amd64`, `windows/amd64`, `darwin/amd64`, `darwin/arm64`.
  The three operating systems are required; the two macOS architectures cover
  Intel and Apple Silicon.
- The default branch is `main`. Releases come only from `main`. The current
  work branch (`improvements`) is not released until it is merged.
- No license file exists in the repository yet. The Scoop manifest carries
  `NOASSERTION` until one is chosen.

## Approaches

1. **Copy zwiftboard as-is** (Windows-only exe). Rejected: it does not meet the
   three-platform requirement.
2. **GoReleaser.** Rejected: a second build system beside `just`, while the
   requirement is to reuse the zwiftboard `justfile` approach.
3. **Chosen: zwiftboard model extended.** `just release VERSION` builds every
   platform into `dist/`, semantic-release calls it via `prepareCmd`, and the
   Scoop manifest is rendered by `just scoop-manifest` from the Windows
   checksum.

## Design

### Version injection

`justfile` already passes `-X .../internal/version.Version` and `.Commit` with
`-ldflags`. Release builds pass the semantic-release version
(`${nextRelease.version}`, bare, no `v`). The TUI header and `--version` already
read `internal/version`, so no application code changes.

### Artifacts (`just release VERSION`)

Written to `dist/`:

- `whoosh2connect.exe`: raw Windows binary, referenced by the Scoop manifest.
- `whoosh2connect-windows-amd64.zip`
- `whoosh2connect-linux-amd64.tar.gz`
- `whoosh2connect-darwin-amd64.tar.gz`
- `whoosh2connect-darwin-arm64.tar.gz`
- `checksums.txt`: sha256 of every file above.

### CI

- `.github/workflows/ci.yml`: on pull requests and from `release.yml`. Runs
  `just ci`: `fmt-check`, `vet`, `test -race`, `build` for the host.
- `.github/workflows/release.yml`: on push to `main` and `dev`. Jobs: `checks`
  (reuses `ci.yml`), `release` (semantic-release; `main` publishes stable
  `X.Y.Z`, `dev` publishes `X.Y.Z-dev.N` prereleases on the `dev` channel),
  `bucket` (renders and commits the Scoop manifest to
  `bucket/whoosh2connect.json`, `main` only, when a new release was published).

### Scoop

`just scoop-manifest VERSION SHA256` prints the manifest for the Windows exe.
A Go test checks it renders valid JSON, uses literal values, and that the
committed `bucket/whoosh2connect.json` is not stale.

### Version display

Already present: `shortVersion()` in `internal/tui/views.go` shows
`X.Y.Z (commit sha)` in the header when built by release, `dev (none)` otherwise.

## Decisions taken without asking (your call to change)

- Architectures: amd64 for Linux and Windows; amd64 and arm64 for macOS.
- Archive format: `.zip` for Windows, `.tar.gz` for Linux and macOS.
- `tagFormat` is `${version}` (no `v`), as in zwiftboard.
- `NOASSERTION` license until one is chosen.

## Left to the builder

- Exact semantic-release plugin versions (pinned as in zwiftboard).
- Exact GitHub Actions versions (copied from zwiftboard).
