# SPDX-FileCopyrightText: 2026 Sebastien Rousseau
# SPDX-License-Identifier: MIT OR Apache-2.0
#
# Template. Base images are digest-pinned, which is what OpenSSF Scorecard
# checks for and what stops a rebuild silently picking up a different base.
#
#   docker build -f pkg/docker/Dockerfile -t draft:local .
#   docker run --rm -v "$PWD:/work" -w /work draft:local --dry-run paper.pdf
#
# Note the tool is only useful in a container with a backend reachable: either
# mount an agent CLI's credentials, or point OLLAMA_HOST at a running server.

FROM golang:1.24-bookworm@sha256:REPLACE_WITH_DIGEST AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X main.version=${VERSION}" \
      -o /out/draft ./cmd/draft \
 && /out/draft --man > /out/draft.1

# Poppler is the reason this is not a scratch image: pdftotext is a real
# runtime dependency for the tool's main input format.
FROM debian:bookworm-slim@sha256:REPLACE_WITH_DIGEST
RUN apt-get update \
 && apt-get install -y --no-install-recommends poppler-utils ca-certificates \
 && rm -rf /var/lib/apt/lists/*

COPY --from=build /out/draft /usr/bin/draft
COPY --from=build /out/draft.1 /usr/share/man/man1/draft.1
COPY LICENSE-MIT LICENSE-APACHE /usr/share/licenses/draft/

# Never root: the tool reads documents it did not write.
RUN useradd --create-home --uid 10001 draft
USER draft
WORKDIR /work

ENTRYPOINT ["/usr/bin/draft"]
CMD ["--help"]
