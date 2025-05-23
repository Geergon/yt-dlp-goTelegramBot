FROM golang:1.24.3-alpine

WORKDIR /yt-dlp-goTelegramBot

COPY go.* ./ 

RUN go mod download

RUN apk --no-cache add \
  jq \
  curl \
  yt-dlp \
  bash \
  busybox-suid \
  dumb-init \
  cronie

RUN curl -L https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp \
  -o /usr/local/bin/yt-dlp && chmod +x /usr/local/bin/yt-dlp

RUN echo "0 3 * * * curl -sSL https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp -o /usr/local/bin/yt-dlp && chmod +x /usr/local/bin/yt-dlp" > /etc/crontabs/root

COPY . .

RUN go build -o main main.go

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/usr/bin/dumb-init", "--"]
CMD ["/entrypoint.sh"]
