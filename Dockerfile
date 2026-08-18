FROM golang:1.24.3-alpine AS builder
WORKDIR /app
COPY go.* ./ 
RUN go mod download
COPY . .
RUN go build -o main main.go

FROM debian:bookworm-slim
COPY updatebot ./
COPY --from=builder /app/main /app

RUN apt-get update && apt-get install -y --no-install-recommends \
  ffmpeg \
  jq curl \
  bash \
  nodejs \
  python3 python3-pip \
  dumb-init \
  cron \
  ca-certificates \
  && pip3 install --break-system-packages gallery-dl \
  && curl -L https://github.com/yt-dlp/yt-dlp-nightly-builds/releases/latest/download/yt-dlp_linux \
  -o /usr/local/bin/yt-dlp && chmod +x /usr/local/bin/yt-dlp \
  && apt-get purge -y --auto-remove \
  && rm -rf /var/lib/apt/lists/*

RUN echo "0 5 * * * /updatebot" | crontab -

ENTRYPOINT ["/usr/bin/dumb-init", "--"]
CMD [ "/app" ]


