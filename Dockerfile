FROM golang:1.26-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN sh build.sh

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata \
    && mkdir -p /app/cache /app/log

WORKDIR /app

COPY --from=builder /build/Music163bot-Go .

CMD [ "./Music163bot-Go" ]
