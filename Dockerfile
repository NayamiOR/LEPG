# Stage 1: Build
FROM golang:1.26-alpine AS builder

# 国内构建加速（默认 proxy.golang.org 在国内不可达）
ENV GOPROXY=https://goproxy.cn,direct

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/server ./cmd/server

# Stage 2: Runtime
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=builder /out/server .

EXPOSE 8883 1883 8083

ENTRYPOINT ["./server", "run"]
