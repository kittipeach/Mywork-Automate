# syntax=docker/dockerfile:1
#
# automate-api — Go/Gin REST control plane (docs/spec/04 §2.2).
#
# The API entrypoint (./apps/automate-api/cmd/api) imports gin only and is pure
# Go, so it is built with CGO_ENABLED=0 and shipped on the distroless *static*
# image. Listens on :8080 and exposes GET /healthz and GET /readyz.
#
# BUILD CONTEXT MUST BE THE REPO ROOT (single Go module github.com/mywork/automate,
# go.work `use .`). From the repo root:
#   docker build -f apps/automate-api/Dockerfile -t automate-api:dev .

# ---- builder ----------------------------------------------------------------
FROM golang:1.22-bookworm AS builder

WORKDIR /src

# Cache module downloads: copy manifests first.
COPY go.mod go.sum go.work ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

# Copy the rest of the source tree.
COPY . .

# Pure Go: no cgo, static binary, stripped, reproducible.
ARG TARGETOS=linux
ARG TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" \
      -o /out/automate-api ./apps/automate-api/cmd/api

# ---- runtime ----------------------------------------------------------------
# static-debian12 has no libc; fine because the binary is CGO_ENABLED=0.
# :nonroot runs as uid/gid 65532.
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /
COPY --from=builder /out/automate-api /usr/local/bin/automate-api

EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/automate-api"]
