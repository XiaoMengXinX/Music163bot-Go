FROM golang:1.26-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN sh build.sh

FROM alpine:3.21

# ca-certificates 访问 Telegram/网易云 API；ffmpeg 供 /recognize 语音识别转码
RUN apk add --no-cache ca-certificates ffmpeg tzdata \
    && addgroup -S bot && adduser -S bot -G bot \
    && mkdir -p /app/cache /app/log && chown -R bot:bot /app

WORKDIR /app

COPY --from=builder --chown=bot:bot /build/Music163bot-Go .

USER bot

CMD [ "./Music163bot-Go" ]
