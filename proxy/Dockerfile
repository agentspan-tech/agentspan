# Build stage
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder
ARG TARGETARCH
WORKDIR /app

# Download dependencies first (layer cache)
COPY proxy/go.mod proxy/go.sum ./
RUN go mod download

# Build binary
COPY proxy/ .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -ldflags="-s -w" -o /proxy ./cmd/proxy/

# Final stage — minimal image
FROM alpine:3.21@sha256:c3f8e73fdb79deaebaa2037150150191b9dcbfba68b4a46d70103204c53f4709
RUN apk add --no-cache ca-certificates wget && \
    addgroup -S app && adduser -S app -G app
COPY --from=builder /proxy /proxy
USER app
ENTRYPOINT ["/proxy"]
