FROM golang:1.15.5-alpine3.12 AS builder

WORKDIR /app
COPY . /app

RUN go build

FROM alpine:3.12

COPY --from=builder /app/proxy-init /usr/bin/proxy-init
ENTRYPOINT ["proxy-init"]
