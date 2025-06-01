FROM golang:1.24.1-alpine3.21

WORKDIR /app

# Copy go.mod and go.sum to download dependencies
COPY ./go.mod ./go.sum ./
RUN go mod download

COPY ./ ./

# Build the Go application
RUN CGO_ENABLED=0 GOOS=linux go build -o /docker-gs-ping ./cmd/app/main.go

# Run the application
ENTRYPOINT ["/docker-gs-ping"]