FROM golang:1.17-alpine AS build

RUN adduser -u 10001 -D -H scratchuser

WORKDIR /app

COPY . .
RUN go mod download
RUN go build -o /boiler-mate

FROM scratch

WORKDIR /

COPY --from=build /boiler-mate /boiler-mate
COPY --from=build /etc/passwd /etc/passwd

EXPOSE 2112
USER scratchuser:scratchuser

ENV BOILER_MATE_METRICS "0.0.0.0:2112"

ENTRYPOINT ["/boiler-mate"]
