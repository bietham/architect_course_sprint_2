# syntax=docker/dockerfile:1.6

# 1) builder запускаем на платформе билдера (amd64 на GitHub, arm64 у тебя на Mac)
FROM --platform=$BUILDPLATFORM golang:1.23-alpine AS build
WORKDIR /src

# Кэш модулей
COPY go.mod go.sum ./
RUN go mod download

# Исходники
COPY . .

# 2) ВАЖНО: кросс-компиляция под целевую платформу от buildx
ARG TARGETOS TARGETARCH
ENV CGO_ENABLED=0
# Если main.go в корне — оставь ".", если в поддиректории — укажи путь (например ./cmd/monolith)
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -ldflags="-s -w" -o /out/monolith .

# 3) Минимальный рантайм (мультиарховый)
FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/monolith /monolith
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/monolith"]
