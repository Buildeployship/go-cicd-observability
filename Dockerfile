# Build stage
FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /relay ./cmd/relay

# Runtime stage
FROM gcr.io/distroless/static-debian12

COPY --from=builder /relay /relay

EXPOSE 8080

ENTRYPOINT ["/relay"]
