# --- Build stage ---
FROM golang:1.22-alpine AS builder
WORKDIR /src

# 1) Модуль
COPY go.mod ./

# 2) Код (после этого tidy соберёт go.sum по реальным импортам)
COPY src ./src

# 3) Сгенерировать go.sum и подтянуть зависимости
#   (tidy пройдётся по ./src, добавит нужные зависимости в go.mod/go.sum)
RUN go mod tidy

# 4) Собрать бинарь
WORKDIR /src/src
ENV CGO_ENABLED=0 GOOS=linux
RUN go build -trimpath -ldflags "-s -w" -o /out/events

# --- Runtime stage ---
FROM gcr.io/distroless/static:nonroot
WORKDIR /app
COPY --from=builder /out/events /app/events
ENV PORT=8082
EXPOSE 8082
USER nonroot:nonroot
ENTRYPOINT ["/app/events"]