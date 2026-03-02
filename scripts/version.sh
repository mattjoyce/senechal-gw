#!/bin/sh
# Derives the ductile version from git state.
# Format: v0.<commit-count>-<short-hash>
# No manual versioning required — runs identically locally and in CI/Docker.
echo "v0.$(git rev-list --count HEAD)-$(git rev-parse --short HEAD)"
