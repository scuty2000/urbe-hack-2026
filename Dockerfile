FROM node:22-bookworm AS web
WORKDIR /web
COPY webui/package*.json ./
RUN npm ci
COPY webui/ ./
RUN npm run build

FROM golang:1.26-bookworm AS build
RUN apt-get update && apt-get install -y --no-install-recommends gcc libc6-dev
WORKDIR /src
COPY . .
COPY --from=web /web/dist ./webui/dist
RUN CGO_ENABLED=1 go build -tags webui -o /out/webapi ./cmd/webapi

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build /out/webapi .
COPY db ./db
COPY demo ./demo
ENV API_DB_FILENAME=/data/lostfound.db API_STORAGE_DATA_DIR=/data
EXPOSE 3000
CMD ["./webapi", "-c", "demo/config.yml"]
