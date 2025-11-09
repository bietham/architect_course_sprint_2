# syntax=docker/dockerfile:1.6

# --- Build stage ---
FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS build
WORKDIR /work

# 1) Модуль 
COPY go.mod ./ 
# 2) Код (после этого tidy соберёт go.sum по реальным импортам) 
COPY src ./src

RUN go mod tidy

# кросс-параметры от buildx
ARG TARGETOS TARGETARCH
ENV CGO_ENABLED=0

# Если main.go лежит в ./src — оставляем так. Если другой путь — подставь его.
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w" -o /out/events ./src

# --- Runtime stage ---
FROM gcr.io/distroless/static:nonroot
WORKDIR /app
COPY --from=build /out/events /app/events
ENV PORT=8082
EXPOSE 8082
USER nonroot:nonroot
ENTRYPOINT ["/app/events"]
