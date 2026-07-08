# ---- builder: compile a static binary (pure-Go deps, no CGO) ----
FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
        -o /out/usage-gauge ./cmd/usage-gauge \
 && mkdir -p /out/config

FROM alpine:latest
RUN apk add --no-cache bash ca-certificates tzdata
COPY --from=builder /out/ /app/
ENV CONFIG_DIR=/app/config \
    REFRESH_INTERVAL_MS=300000 \
    PORT=3000
EXPOSE 3000
ENTRYPOINT ["/app/usage-gauge"]
