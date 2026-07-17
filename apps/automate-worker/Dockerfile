# syntax=docker/dockerfile:1
#
# automate-worker — Go Temporal worker (docs/spec/04 §2.3).
#
# The worker entrypoint (./apps/automate-worker/cmd/worker) transitively imports
# pkg/sqlguard, which uses cgo via github.com/pganalyze/pg_query_go/v6. CGO IS
# REQUIRED, so it is built with CGO_ENABLED=1 in a gcc-equipped builder and
# shipped on the distroless *base* image (which ships glibc). No HTTP port.
#
# BUILD CONTEXT MUST BE THE REPO ROOT (single Go module github.com/mywork/automate,
# go.work `use .`). From the repo root:
#   docker build -f apps/automate-worker/Dockerfile -t automate-worker:dev .

# ---- builder ----------------------------------------------------------------
# bookworm ships gcc + glibc headers, required for the pg_query_go cgo build.
FROM golang:1.22-bookworm AS builder

WORKDIR /src

# Cache module downloads: copy manifests first.
COPY go.mod go.sum go.work ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

# Copy the rest of the source tree.
COPY . .

# cgo required. Do NOT set GOARCH from a cross ARG here — cgo cross-compilation
# needs a matching cross toolchain we do not install; build for the builder's
# native arch, which matches the target under `kind load` on the same host.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 GOOS=linux \
    go build -trimpath -ldflags="-s -w" \
      -o /out/automate-worker ./apps/automate-worker/cmd/worker

# ---- runtime ----------------------------------------------------------------
# base-debian12 ships glibc + ca-certificates, required because the binary is
# dynamically linked (CGO_ENABLED=1). :nonroot runs as uid/gid 65532.
FROM gcr.io/distroless/base-debian12:nonroot

WORKDIR /
COPY --from=builder /out/automate-worker /usr/local/bin/automate-worker

USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/automate-worker"]
