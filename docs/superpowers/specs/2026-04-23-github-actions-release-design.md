# GitHub Actions Release Design

## Goal

Add a manual GitHub Actions release workflow that builds Linux `amd64` and `arm64` artifacts, creates a Git tag and GitHub release, and uploads packaged release assets without changing the existing local `build.sh` behavior.

## Current State

- `build.sh` runs `go mod tidy`, builds `linux/amd64` and `linux/arm64` binaries into `dist/`, and copies `install-systemd-service.sh`
- The repository does not contain a `.github/workflows/` directory or any release automation
- `install-systemd-service.sh` expects the executable in the same directory and looks for `media-backup-linux-amd64` or `media-backup-linux-arm64`
- Existing tests in `cmd/media-backup/install_script_test.go` assert those binary names

## Requirements

1. Keep the current `build.sh` file unchanged for local use.
2. Add a separate CI-only build script for release packaging.
3. Release builds must produce two archives:
   - `media-backup_<version>_linux_amd64.tar.gz`
   - `media-backup_<version>_linux_arm64.tar.gz`
4. Each archive must contain exactly:
   - the matching architecture binary named `media-backup-linux-<arch>`
   - `install-systemd-service.sh`
5. The GitHub Actions workflow must be manually triggered with a required `version` input such as `v0.3.0`.
6. The workflow must run tests before building release assets.
7. The workflow must create the Git tag and GitHub release and upload both archives to that release.
8. Releases are published as normal releases, not prereleases.

## Versioning Recommendation

Use semantic version tags in `vX.Y.Z` form.

- Near-term recommendation: stay on `v0.x.y` while deployment and operational behavior are still evolving
- Patch releases (`v0.3.1`) are for fixes and packaging-only corrections
- Minor releases (`v0.4.0`) are for new features or behavior changes
- Delay `v1.0.0` until installation, configuration, and runtime behavior are considered stable enough to support compatibility expectations

## Design

### CI Build Script

Add a new top-level script `ci_build.sh` dedicated to release packaging.

Responsibilities:

- Require a non-empty `VERSION` value
- Normalize the archive version segment by removing the leading `v` from `VERSION`
- Recreate `dist/` from scratch for deterministic release output
- Build `linux/amd64` and `linux/arm64` binaries with the same output names currently used by the installer
- Stage each architecture into its own temporary packaging directory
- Copy `install-systemd-service.sh` into each package directory and keep it executable
- Produce one `.tar.gz` archive per architecture under `dist/`

Non-responsibilities:

- It should not replace or modify `build.sh`
- It should not run `go mod tidy`
- It should not attempt to publish releases itself

### Archive Layout

Each generated archive should unpack into a flat directory containing:

- `media-backup-linux-amd64` or `media-backup-linux-arm64`
- `install-systemd-service.sh`

The archive should not contain both architectures together because the installer selects a single expected binary name based on host architecture. Keeping one binary per archive avoids ambiguity and preserves the current install script contract.

### GitHub Actions Workflow

Add `.github/workflows/release.yml` with a `workflow_dispatch` trigger and one required input:

- `version`: full release tag, for example `v0.3.0`

Workflow behavior:

1. Check out the repository with permissions that allow pushing a tag and creating a release.
2. Install the Go toolchain from the repository's `go.mod` version.
3. Run `go test ./...` and stop immediately on failure.
4. Run `VERSION=${{ inputs.version }} ./ci_build.sh`.
5. Create and push the Git tag named by the input version.
6. Create a GitHub release using the same version string as the tag and release title.
7. Upload the two generated archives from `dist/` to that release.

### Failure Behavior

- If tests fail, the workflow fails before any tag or release is created.
- If the requested tag already exists, the workflow should fail rather than overwrite or mutate an existing release.
- If asset upload fails after release creation, the workflow should fail visibly so the partially created release can be corrected explicitly instead of being silently reused.

## Testing

Add targeted tests for the new packaging behavior in a dedicated shell-oriented Go test file alongside the existing install-script tests.

Test coverage should include:

1. `ci_build.sh` fails when `VERSION` is unset
2. `ci_build.sh` creates both expected archive filenames for a valid version
3. Each archive contains the matching binary name and `install-systemd-service.sh`
4. The archive contents do not include the opposite architecture binary

The GitHub Actions workflow itself does not need repository-local automated tests, but the script it calls should be covered so the release path is not only exercised in CI.

## Out of Scope

- Automatic changelog generation
- Automatic release notes based on commits
- Non-Linux release targets
- Nightly builds or snapshot publishing
- Automatic release triggering from branch pushes or tags
