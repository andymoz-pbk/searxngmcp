FROM golang:1.23-alpine AS builder
WORKDIR /build
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o searxngmcp .

FROM scratch
COPY --from=builder /build/searxngmcp /searxngmcp
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY config.example.json /etc/searxngmcp/config.json
EXPOSE 8000
ENTRYPOINT ["/searxngmcp"]
CMD ["--config", "/etc/searxngmcp/config.json"]
