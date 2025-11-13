# Go Vendor Setup for Faster Builds

## Overview

This project uses Go vendor directory to cache dependencies locally, significantly speeding up the `make generate` process by avoiding repeated downloads during Docker builds.

## Benefits

- **Faster Builds**: No need to download dependencies each time you run `make generate`
- **Offline Builds**: Can build without internet connection (except for go-bindata tool)
- **Consistent Dependencies**: All team members use the same vendored dependencies
- **Avoids Toolchain Download**: `GOTOOLCHAIN=local` prevents downloading Go 1.24.9 in container

## Initial Setup

Run once to create the vendor directory:

```bash
go mod download
go mod vendor
```

This downloads all dependencies from `go.mod` into the `vendor/` directory.

## Keeping Vendor Up-to-Date

When you update dependencies in `go.mod`, refresh the vendor directory:

```bash
# After adding/updating dependencies
go mod tidy
go mod vendor
```

## How It Works

### Dockerfile Changes

The optimized `Dockerfile.openapi` now:

1. **Copies vendor first** (lines 11-12):
   ```dockerfile
   COPY vendor /local/vendor
   ```

2. **Sets GOTOOLCHAIN=local** (line 22):
   ```dockerfile
   ENV GOTOOLCHAIN=local
   ```
   This prevents downloading Go 1.24.9 toolchain specified in go.mod

3. **Uses vendor for go generate** (line 34):
   ```dockerfile
   RUN go generate -mod=vendor /local/cmd/hyperfleet/main.go
   ```

### What Still Downloads

- **go-bindata tool** (line 29): This is a build tool, not a library dependency
  - Small download (~1-2 MB)
  - Cached in Docker layers after first build

### What No Longer Downloads

- ✅ All Go module dependencies (now from vendor/)
- ✅ Go 1.24.9 toolchain (uses local Go 1.21 via GOTOOLCHAIN=local)

## Git Workflow

The `vendor/` directory **is committed to git** (not in .gitignore), so:

- Pull requests include vendored dependencies
- Team members don't need to run `go mod vendor` unless changing dependencies
- CI/CD builds are faster and more reliable

## Troubleshooting

### "missing go.sum entry" error

```bash
go mod tidy
go mod vendor
```

### Vendor out of sync with go.mod

```bash
rm -rf vendor
go mod vendor
```

### Want to verify vendor is complete

```bash
go mod verify
```

## Size Considerations

The vendor directory adds to repository size. Current size:

```bash
du -sh vendor/
```

This is a trade-off between:
- **Repository size** vs **Build speed and reliability**

For most teams, the build speed improvement is worth it.

## Alternative: Not Using Vendor

If you prefer not to commit vendor/:

1. Add `vendor/` to `.gitignore`
2. Remove vendor-related lines from `Dockerfile.openapi`
3. Each developer runs `go mod download` before building
4. Builds will be slower but repository stays smaller
