FROM docker:27-cli AS docker-cli

FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /api ./cmd/animeup

FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=docker-cli /usr/local/bin/docker /usr/local/bin/docker
COPY --from=builder /api /api
EXPOSE 4751
ENTRYPOINT ["/api", "serve"]
