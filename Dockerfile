FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY . .

ENV CGO_ENABLED=0
RUN apk add -U --no-cache ca-certificates && update-ca-certificates
RUN go mod download
RUN go build -o /boiler-mate ./cmd/boiler-mate

FROM scratch
WORKDIR /
COPY --from=builder /boiler-mate ./
COPY --from=builder /etc/ssl/certs/ /etc/ssl/certs

EXPOSE 2112
USER 10001:10001

ENV BOILER_MATE_METRICS="0.0.0.0:2112"

ENTRYPOINT ["/boiler-mate"]
