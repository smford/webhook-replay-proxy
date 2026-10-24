# Build stage
FROM golang:1.25-alpine@sha256:1ae0735f00daffa3aaf1363a5184c0d2dc55c78e3db4ec70241cdac97bf84b59 AS builder

WORKDIR /src

# Download dependencies first for caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build statically linked binary
ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -ldflags="-s -w" -o /bin/webhook-replay-proxy ./cmd/webhook-replay-proxy

# Runtime stage
FROM alpine:3.21@sha256:21c2e2e20be64c44b13d8f63863c5c0d40448247af8216fa1f4369e1b29a54f0

# Install ca-certificates and tzdata
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user and configuration directory
RUN addgroup -S webhook-replay-proxy && adduser -S webhook-replay-proxy -G webhook-replay-proxy && \
    mkdir -p /etc/webhook-replay-proxy && \
    chown -R webhook-replay-proxy:webhook-replay-proxy /etc/webhook-replay-proxy

COPY --from=builder /bin/webhook-replay-proxy /usr/local/bin/webhook-replay-proxy

USER webhook-replay-proxy
WORKDIR /etc/webhook-replay-proxy

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget -q -O- http://127.0.0.1:8080/healthz || exit 1

ENTRYPOINT ["/usr/local/bin/webhook-replay-proxy"]
CMD ["-config", "/etc/webhook-replay-proxy/webhook-replay-proxy.yaml"]
