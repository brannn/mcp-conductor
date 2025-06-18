# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /workspace

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binaries
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -o controller ./cmd/controller
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -o agent ./cmd/agent
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -o mcp-server ./cmd/mcp-server

# Final stage
FROM gcr.io/distroless/static:nonroot

WORKDIR /app

# Copy the binaries from builder stage
COPY --from=builder /workspace/controller /app/controller
COPY --from=builder /workspace/agent /app/agent
COPY --from=builder /workspace/mcp-server /app/mcp-server

# Use nonroot user
USER 65534:65534

# Default to running the controller
ENTRYPOINT ["/app/controller"]
