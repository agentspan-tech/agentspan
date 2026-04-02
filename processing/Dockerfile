# Frontend build stage
FROM node:22-alpine@sha256:4d64b49e6c891c8fc821007cb1cdc6c0db7773110ac2c34bf2e6960adef62ed3 AS frontend
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ .
RUN npm run build

# Go build stage
FROM --platform=$BUILDPLATFORM golang:1.26-alpine@sha256:2389ebfa5b7f43eeafbd6be0c3700cc46690ef842ad962f6c5bd6be49ed82039 AS builder
ARG TARGETARCH
WORKDIR /app

# Copy processing module
COPY processing/go.mod processing/go.sum ./
RUN go mod download

# Copy processing source
COPY processing/ .

# Copy freshly built frontend into the embed path
COPY --from=frontend /web/dist ./web/dist

# Binary is built with the embedded frontend
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -ldflags="-s -w" -o /processing ./cmd/processing/

# Final stage — minimal image
FROM alpine:3.21@sha256:c3f8e73fdb79deaebaa2037150150191b9dcbfba68b4a46d70103204c53f4709
RUN apk add --no-cache ca-certificates wget && \
    addgroup -S app && adduser -S app -G app
COPY --from=builder /processing /processing
USER app
ENTRYPOINT ["/processing"]
