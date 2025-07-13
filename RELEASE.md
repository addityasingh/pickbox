# Release Process

This document describes how to create releases for the Pickbox distributed storage system.

## Overview

The GitHub Actions workflow has been updated to properly handle releases using tags instead of attempting to create releases on main branch pushes, which was causing 403 permission errors.

## How to Create a Release

### 1. Ensure Main Branch is Ready
```bash
# Make sure main branch has all the changes you want to release
git checkout main
git pull origin main
```

### 2. Create and Push a Tag
```bash
# Create a new tag (use semantic versioning)
git tag v1.0.0

# Push the tag to trigger the release workflow
git push origin v1.0.0
```

### 3. Monitor the Release Process
- Go to the Actions tab in GitHub to monitor the workflow
- The release workflow will only trigger on tag pushes, not regular commits
- Artifacts from the latest main branch build are available for 30 days

## Tag Naming Convention

- **Stable releases**: `v1.0.0`, `v1.1.0`, `v2.0.0` (will be marked as stable releases)
- **Pre-releases**: `v1.0.0-beta.1`, `v1.0.0-rc.1` (will be marked as pre-releases)
- **Development tags**: `dev-v1.0.0` (will be marked as pre-releases)

## What Happens During Release

1. **Tests Run**: All unit tests, benchmarks, and security scans
2. **Builds Created**: Binaries for Linux, Windows, and macOS (amd64 and arm64)
3. **Release Created**: GitHub release with all binaries attached
4. **Artifacts**: All build artifacts are attached to the release

## Development Builds

For development purposes, every push to `main` creates development artifacts that are stored for 30 days. These can be downloaded from the Actions tab without creating a formal release.

## Permissions

The workflow now includes explicit permissions:
- `contents: write` - Required to create releases and upload assets

## Troubleshooting

### 403 Errors
- **Old Issue**: Trying to create releases on main branch pushes
- **Solution**: Only create releases via git tags
- **Check**: Ensure repository has Actions permissions enabled

### Missing Binaries
- **Check**: Look at the build job logs to see if all platforms built successfully
- **Common Issue**: Windows ARM64 builds are excluded due to Go compatibility

### Failed Tests
- **Requirement**: All tests must pass before a release can be created
- **Solution**: Fix any failing tests before tagging a release

## Example Release Workflow

```bash
# 1. Finish your development work on main
git checkout main
git pull origin main

# 2. Update version information if needed
# Edit relevant files (README.md, etc.)

# 3. Commit any version bumps
git add .
git commit -m "Prepare release v1.0.0"
git push origin main

# 4. Create and push the release tag
git tag v1.0.0
git push origin v1.0.0

# 5. Monitor the GitHub Actions workflow
Go to: https://github.com/addityasingh/pickbox/actions
```

## Release Notes

The release workflow automatically generates release notes with:
- Feature highlights
- Binary download links
- Test coverage information
- Build commit SHA

For more detailed release notes, you can edit the release after it's created on GitHub. 