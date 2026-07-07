# syntax=docker/dockerfile:1

# ---- builder: compile a static binary (pure-Go deps, no CGO) ----
FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
        -o /out/usage-gauge ./cmd/usage-gauge \
 && mkdir -p /out/config

# ---- runner: minimal image, no shell ----
# Runs as root so it can write to a host-mounted /app/config regardless of the
# directory owner (convenient for root deployments). For a non-root deployment,
# use the :nonroot image, restore `USER nonroot:nonroot`, and on the host run
# `chown -R 65532:65532` on the mounted config directory.
FROM gcr.io/distroless/static-debian12 AS runner
COPY --from=builder /out/ /app/
ENV CONFIG_DIR=/app/config \
    REFRESH_INTERVAL_MS=300000 \
    PORT=3000
EXPOSE 3000
ENTRYPOINT ["/app/usage-gauge"]
