FROM golang:1.16-alpine AS builder
ENV GO111MODULE=on \
    CGO_ENABLED=0
WORKDIR /src
ADD . .
RUN go build -o ./bin/mqtt2timescaledb

FROM alpine:3.12
COPY --from=builder /src/bin/mqtt2timescaledb /app/mqtt2timescaledb
RUN apk add --no-cache tini
WORKDIR /app
ENTRYPOINT ["/sbin/tini", "--"]
CMD ["./mqtt2timescaledb"]
