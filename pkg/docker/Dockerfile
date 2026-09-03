# SPDX-FileCopyrightText: 2026 Sebastien Rousseau
# SPDX-License-Identifier: MIT OR Apache-2.0
#
# Template. Base images are digest-pinned, which is what OpenSSF Scorecard
# checks for and what stops a rebuild silently picking up a different base.
# The tag is kept beside the digest so a reader can see which release it is;
# the digest is what actually resolves. Refresh both together with:
#   docker buildx imagetools inspect golang:1.24-bookworm --format '{{.Manifest.Digest}}'
#
#   docker build -f pkg/docker/Dockerfile -t draft:local .
#   docker run --rm -v "$PWD:/work" -w /work draft:local --dry-run paper.pdf
#
# Note the tool is only useful in a container with a backend reachable: either
# mount an agent CLI's credentials, or point OLLAMA_HOST at a running server.

FROM golang:1.24-bookworm@sha256:1a6d4452c65dea36aac2e2d606b01b4a029ec90cc1ae53890540ce6173ea77ac AS build
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
FROM debian:bookworm-slim@sha256:88200866dfff7ea7f5cbcb6ec7c8a701889efe6fe859fe64d6990e4b07ea4171
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
