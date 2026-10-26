# Build stage
FROM golang:1.27-alpine@sha256:738d1cf061836894ff6bb8c33881080ac66de8cf0586615012a0c8f592649cfa AS builder

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
FROM alpine:3.24@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a

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
