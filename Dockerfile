FROM golang:1.23-alpine AS builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -o portal .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates sqlite
WORKDIR /app
COPY --from=builder /app/portal .
RUN mkdir -p /data/uploads
VOLUME /data
EXPOSE 8080
CMD ["./portal", "/data/config.json"]
