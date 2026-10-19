FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM golang:1.24-alpine AS server
WORKDIR /src
COPY go.mod go.sum ./
COPY cmd/ cmd/
COPY internal/ internal/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /bilifetch ./cmd/bilifetch

FROM python:3.12-slim-bookworm
RUN apt-get update && apt-get install -y --no-install-recommends ffmpeg ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && pip install --no-cache-dir yt-dlp==2026.8.19 \
    && useradd --uid 10001 --create-home bilifetch
WORKDIR /app
COPY --from=server /bilifetch /usr/local/bin/bilifetch
COPY --from=web /src/web/dist /app/web/dist
RUN mkdir -p /app/data /app/downloads && chown -R bilifetch:bilifetch /app
ENV BILIFETCH_ADDR=0.0.0.0:8090 BILIFETCH_DATA=/app/data BILIFETCH_DOWNLOADS=/app/downloads BILIFETCH_WEB=/app/web/dist
USER bilifetch
EXPOSE 8090
HEALTHCHECK --interval=30s --timeout=5s CMD python -c "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8090/api/health',timeout=3)" || exit 1
CMD ["bilifetch"]
