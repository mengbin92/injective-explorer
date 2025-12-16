# Build stage
# Use standard golang image (Debian-based with glibc) instead of alpine
# because wasmvm library requires glibc (libwasmvm.x86_64.so is compiled with glibc)
FROM golang:1.25 AS builder

# Install build dependencies for CGO
RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    make \
    gcc \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Set working directory
WORKDIR /src

# Set GOPROXY to use goproxy.cn
ENV GOPROXY=https://goproxy.cn

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application with CGO enabled
# secp256k1 requires CGO to be enabled
ENV CGO_ENABLED=1
RUN make build

# Find and copy wasmvm shared libraries for runtime
# wasmvm package includes pre-compiled shared libraries
RUN mkdir -p /src/lib && \
    find /go/pkg/mod -name "libwasmvm.*.so" -type f -exec cp {} /src/lib/ \;

# Runtime stage
# Use Debian slim instead of alpine because the binary may depend on glibc
# (especially if linked with wasmvm shared libraries)
FROM debian:stable-slim

# Install CA certificates and timezone data
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    tzdata \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user for security
RUN groupadd -g 1000 appuser && \
    useradd -u 1000 -g appuser -s /bin/sh -m appuser

# Set working directory
WORKDIR /app

# Copy binary from builder stage
# The build process creates executables in bin/ directory
COPY --from=builder /src/bin ./bin

# Copy openapi.yaml file for Swagger UI
# This file is needed for the Swagger documentation endpoint
COPY --from=builder /src/openapi.yaml ./openapi.yaml

# Copy wasmvm shared libraries from builder stage
# wasmvm package includes pre-compiled shared libraries that need to be available at runtime
RUN mkdir -p /usr/local/lib
COPY --from=builder /src/lib/ /usr/local/lib/

# Set LD_LIBRARY_PATH to include the library directory
# Use ${LD_LIBRARY_PATH:-} to avoid warning when variable is undefined
ENV LD_LIBRARY_PATH=/usr/local/lib${LD_LIBRARY_PATH:+:${LD_LIBRARY_PATH}}

# Change ownership to non-root user
RUN chown -R appuser:appuser /app && \
    chown -R appuser:appuser /usr/local/lib

# Switch to non-root user
USER appuser

# Expose ports
EXPOSE 8000
EXPOSE 9000

# Create volume mount point
VOLUME /data/conf

# Run the application
CMD ["./bin/injective-explorer", "-conf", "/data/conf"]
