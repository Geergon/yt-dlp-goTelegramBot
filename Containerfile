FROM golang:1.24.3-alpine AS builder

WORKDIR /app

COPY go.* ./ 

RUN go mod download

COPY . .

RUN go build -o main main.go

FROM jrottenberg/ffmpeg:6-alpine

COPY updatebot ./


COPY --from=builder /app/main /app

RUN echo "http://dl-cdn.alpinelinux.org/alpine/edge/community" >> /etc/apk/repositories && \
  apk --no-cache add \
  jq curl \
  bash \
  gallery-dl \
  dumb-init \
  && curl -L https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp_musllinux \
  -o /usr/local/bin/yt-dlp && chmod +x /usr/local/bin/yt-dlp

RUN echo "30 3 * * * /updatebot" | crontab -

ENTRYPOINT ["/usr/bin/dumb-init", "--"]
CMD [ "/app" ]


