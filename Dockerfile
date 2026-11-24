FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS builder

WORKDIR /app
ARG TARGETOS
ARG TARGETARCH
ENV CGO_ENABLED=0
RUN apk add --no-cache ca-certificates

COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN GOOS="$TARGETOS" GOARCH="$TARGETARCH" go build -mod=readonly -trimpath -o /boiler-mate ./cmd/boiler-mate \
    && install -d -m 0700 /boiler-mate-data

FROM scratch
COPY --from=builder /boiler-mate /boiler-mate
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder --chown=10001:10001 /boiler-mate-data/ /data/

EXPOSE 2112 8080
USER 10001:10001
ENV BOILER_MATE_BIND=0.0.0.0:2112 BOILER_MATE_WEB_DATA=/data

ENTRYPOINT ["/boiler-mate"]
