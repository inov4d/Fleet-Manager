# Build stage
FROM golang:1.24-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOTOOLCHAIN=local
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/fleetd ./cmd/fleetd

# Runtime stage
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 fleet
WORKDIR /app
COPY --from=build /out/fleetd /app/fleetd
USER fleet
ENV FM_HTTP_ADDR=:8080 FM_MQTT_ADDR=:1883 FM_DATA_DIR=/data
VOLUME ["/data"]
EXPOSE 8080 1883
HEALTHCHECK --interval=15s --timeout=3s CMD wget -qO- http://127.0.0.1:8080/health || exit 1
ENTRYPOINT ["/app/fleetd"]
