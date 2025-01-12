FROM golang:1.23.4-alpine3.21 AS builder

WORKDIR /app

COPY . .

ENV CGO_ENABLED=0
RUN go mod download
RUN go build -o /boiler-mate

FROM scratch
WORKDIR /
COPY --from=build /boiler-mate ./
EXPOSE 2112
USER nobody:nogroup

ENV BOILER_MATE_METRICS="0.0.0.0:2112"

ENTRYPOINT ["/boiler-mate"]
