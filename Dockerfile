FROM golang:1.15.11-alpine3.13 AS builder

WORKDIR /app
COPY . /app

RUN go build

FROM us.gcr.io/celo-org/geth-all:1.2.3

COPY --from=builder /app/proxy-init /usr/bin/proxy-init
ENTRYPOINT ["proxy-init"]
