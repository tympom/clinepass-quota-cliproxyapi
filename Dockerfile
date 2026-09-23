# syntax=docker/dockerfile:1
# Builds the clinepass-quota-cliproxyapi CLIProxyAPI plugin (.so) from the
# local source in this directory. Locally authored code, not a pinned
# upstream commit: reproducibility is verified by a deterministic --no-cache
# rebuild diff instead of an official-release comparison (see build report).
FROM golang:1.27-bookworm AS builder

WORKDIR /src

# Dependencies first for better layer caching.
COPY go.mod go.sum* ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
    go build -trimpath -buildmode=c-shared \
      -ldflags "-s -w" \
      -o /out/clinepass-quota-cliproxyapi.so . \
    && rm -f /out/clinepass-quota-cliproxyapi.h

# Minimal artifact-only stage: no shell tooling needed to run this plugin
# (CLIProxyAPI itself loads the .so from its own container), this stage only
# exists so `docker create` + `docker cp` can extract the artifact as a
# non-root-owned file without invoking the builder's full toolchain image.
FROM gcr.io/distroless/static-debian12:nonroot AS artifact
COPY --from=builder /out/clinepass-quota-cliproxyapi.so /clinepass-quota-cliproxyapi.so
