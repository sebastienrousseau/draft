#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Sebastien Rousseau
# SPDX-License-Identifier: MIT OR Apache-2.0
#
# Bring a fresh container to a working `make`. Anything installed here is
# something DEVELOPMENT.md says you need; if the two disagree, this file is
# wrong.
set -euo pipefail

echo "==> Runtime tools draft shells out to"
sudo apt-get update -qq
sudo apt-get install -y -qq poppler-utils groff

echo "==> Lint and docs tooling"
python3 -m pip install --quiet --disable-pip-version-check \
  codespell mkdocs mkdocs-material pre-commit

# Pinned together with the Go toolchain; see the comment in ci.yml.
echo "==> golangci-lint"
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2

echo "==> Warming the module cache"
go mod download

echo "==> Verifying the container can run the gates"
gofmt -s -l . >/dev/null
go build ./...
echo
echo "Ready. Try:  make check"
