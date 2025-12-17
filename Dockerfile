# -- Build Stage --
    FROM golang:1.25-alpine AS builder

    WORKDIR /src
    
    # Initialize module if you haven't locally (keeps the build self-contained)
    RUN go mod init httspy
    
    # Copy the source code (assuming your file is named main.go)
    COPY main.go .
    
    # Build flags:
    # -ldflags="-s -w": Strips debug symbols (smaller binary)
    # CGO_ENABLED=0: Static binary (no C library dependencies)
    RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o httspy main.go
    
    # -- Final Stage --
    FROM scratch
    
    # Metadata
    LABEL app="httspy"
    LABEL description="Kubernetes Gateway API Request Mirror Sink"
    
    # Copy the binary from builder
    COPY --from=builder /src/httspy /httspy
    
    # Run as a non-root user (User ID 65532 is commonly used for distroless/non-root)
    USER 65532:65532
    
    EXPOSE 8080
    
    ENTRYPOINT ["/httspy"]