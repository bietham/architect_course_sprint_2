# syntax=docker/dockerfile:1.6

# --- Build stage (multi-arch aware) ---
FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS build
WORKDIR /work

# Кэш модулей
COPY go.mod ./ 
RUN go mod download

# Исходники (у тебя код лежит в ./src)
COPY . .

# Кросс-компиляция под целевую платформу от buildx
ARG TARGETOS TARGETARCH
ENV CGO_ENABLED=0
# Если main.go в ./src — оставляем так; поменяй путь при необходимости
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/proxy ./src

# --- Runtime stage (multi-arch) ---
FROM gcr.io/distroless/static:nonroot
WORKDIR /app
COPY --from=build /out/proxy /app/proxy
ENV PORT=8000
EXPOSE 8000
USER nonroot:nonroot
ENTRYPOINT ["/app/proxy"]
