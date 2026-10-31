# Этап 1: сборка веб-панели (React/Vite)
FROM node:20-alpine AS web
WORKDIR /web
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build          # результат -> /web/dist

# Этап 2: сборка статического бинарника
FROM golang:alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /bot ./cmd/bot

# Этап 3: минимальный рантайм без shell, non-root
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /bot /bot
# Собранная веб-панель; путь совпадает с WEB_DIR по умолчанию ниже.
COPY --from=web /web/dist /web
ENV WEB_DIR=/web
USER nonroot
ENTRYPOINT ["/bot"]
