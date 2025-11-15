# syntax=docker/dockerfile:1.6

# --- build ---
FROM --platform=$BUILDPLATFORM golang:1.23-alpine AS build
WORKDIR /src

# кэш модулей
COPY go.mod go.sum ./
RUN go mod download

# исходники
COPY . .

# кросс-сборка под целевую платформу
ARG TARGETOS TARGETARCH
ENV CGO_ENABLED=0
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/movies-service .

# --- runtime ---
FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/movies-service /movies-service
USER nonroot:nonroot
EXPOSE 8081
ENTRYPOINT ["/movies-service"]
