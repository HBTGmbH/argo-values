# Build stage - compile the Go application
FROM golang:1.26-alpine AS builder

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o argo-values cmd/argo-values/main.go

# Final stage - create a minimal image
FROM alpine:latest

ARG HELM_VERSION=3.19.0-r5
ARG APP_VERSION=1.0.0
ARG GO_VERSION=1.26

# Add labels for better image identification
LABEL org.opencontainers.image.title="argo-values"
LABEL org.opencontainers.image.description="CLI tool for Argo Application values"
LABEL org.opencontainers.image.version="${APP_VERSION}"
LABEL org.opencontainers.image.helm.version="${HELM_VERSION}"
LABEL org.opencontainers.image.go.version="${GO_VERSION}"
LABEL maintainer="henning@huhehu.com"

# Create a non-root user for security
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

WORKDIR /home/appuser

# Copy the compiled binary from the builder stage to a system directory
COPY --from=builder /app/argo-values /usr/local/bin/argo-values

# Set proper permissions so all users can execute the binary
RUN chmod +x /usr/local/bin/argo-values && \
    chown root:appgroup /usr/local/bin/argo-values

# Switch to root for package installation
USER root

# Install Helm (required for build command)
# Allow Helm version to be specified via build argument
# To find available versions: https://pkgs.alpinelinux.org/packages?name=helm
RUN apk add --no-cache --upgrade helm=$HELM_VERSION

# Switch back to non-root user for security
USER appuser

# Set up entrypoint
ENTRYPOINT ["argo-values"]

# Set the command to show help by default
CMD ["--help"]