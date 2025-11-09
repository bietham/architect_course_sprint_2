# --- Build stage ---
FROM golang:1.22-alpine AS builder
WORKDIR /src

COPY go.mod ./
COPY src ./src

# важное: формируем go.sum по фактическим импортам (в т.ч. kafka-go)
RUN apk add --no-cache git && \
    cd /src && go mod tidy

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