# CinemaAbyss Proxy (Go, no npm)
Исправленная версия: алиасы для crypto/rand и math/rand, удалён неиспользуемый import.

Папка для `./src/microservices/proxy`:
- Dockerfile
- go.mod
- src/main.go

Сборка:
```
docker compose build proxy-service --no-cache
docker compose up -d proxy-service
```
