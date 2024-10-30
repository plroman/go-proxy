# syntax=docker/dockerfile:1
FROM golang:1.23 AS builder
ARG VERSION
WORKDIR /build
ADD go.mod /build/
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 GOOS=linux \
       go mod download
ADD . /build/
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 GOOS=linux \
    go build \
        -trimpath \
        -ldflags "-s -X main.version=${VERSION}" \
        -v \
        -o receiver-proxy \
    cmd/receiver-proxy/main.go

FROM alpine:latest
WORKDIR /app
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /build/receiver-proxy /app/receiver-proxy
ENV LISTEN_ADDR=":8080"
EXPOSE 8080
CMD ["/app/receiver-proxy"]
