FROM golang:1.20-alpine as builder

WORKDIR /app

COPY go.mod go.sum ./
RUN apk add --no-cache --virtual .build-deps \
        ca-certificates \
        tzdata \
        gcc \
        g++ &&  \
    go mod download

COPY . .

RUN go build -ldflags "-s -w" -o auth

FROM alpine

# Installs latest Chromium package.
RUN apk upgrade --no-cache --available \
    && apk add --no-cache \
      chromium-swiftshader \
      ttf-freefont \
      font-noto-emoji \
    && apk add --no-cache \
      --repository=https://dl-cdn.alpinelinux.org/alpine/edge/testing \
      font-wqy-zenhei

COPY local.conf /etc/fonts/local.conf

WORKDIR /app

COPY --from=builder /app/auth /app/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY data data

ENV TZ=Asia/Shanghai

ENV MODE=production

RUN mkdir -p ./screenshots

VOLUME ["/app/screenshots"]

EXPOSE 8000

ENTRYPOINT ["./auth"]