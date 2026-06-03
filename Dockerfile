FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o wireguard-exporter .

FROM scratch
COPY --from=builder /src/wireguard-exporter /wireguard-exporter
EXPOSE 9586
ENTRYPOINT ["/wireguard-exporter"]
