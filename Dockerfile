FROM golang:1.25-alpine AS builder

RUN apk add --no-cache --update git build-base
ENV CGO_ENABLED=0

WORKDIR /app

COPY . /app

WORKDIR /app/src
ARG BUILD_VERSION=v1.0.10
ARG BUILD_COMMIT=custom
ARG BUILD_DATE=unknown
RUN go mod download
RUN go build -trimpath \
    -ldflags="-s -w \
      -X github.com/GMWalletApp/epusdt/config.BuildVersion=${BUILD_VERSION} \
      -X github.com/GMWalletApp/epusdt/config.BuildCommit=${BUILD_COMMIT} \
      -X github.com/GMWalletApp/epusdt/config.BuildDate=${BUILD_DATE}" \
    -o /app/epusdt .

FROM alpine:3.22 AS runner
ENV TZ=Asia/Shanghai
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app
COPY --from=builder /app/src/.env.example /app/.env
COPY --from=builder /app/epusdt .

VOLUME ["/app/runtime"]
EXPOSE 8000
ENTRYPOINT ["./epusdt", "http", "start"]
