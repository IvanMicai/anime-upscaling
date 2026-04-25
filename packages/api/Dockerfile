FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /api ./cmd/animeup

FROM ghcr.io/k4yt3x/video2x:6.4.0
COPY --from=builder /api /usr/local/bin/api
EXPOSE 4751
ENTRYPOINT ["/usr/local/bin/api", "serve"]
