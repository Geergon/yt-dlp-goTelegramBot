FROM golang:1.24.3-alpine AS builder

WORKDIR /app

COPY go.* ./ 

RUN go mod download

COPY . .

RUN go build -o main main.go

FROM jrottenberg/ffmpeg:8-alpine

COPY updatebot ./

COPY --from=builder /app/main /app

RUN apk --no-cache add \
  jq curl \
  bash \
  python3 py3-pip \
  dumb-init \
  && pip3 install --break-system-packages gallery-dl \
  && curl -L https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp_musllinux \
  -o /usr/local/bin/yt-dlp && chmod +x /usr/local/bin/yt-dlp

RUN echo "0 5 * * * /updatebot" | crontab -

ENTRYPOINT ["/usr/bin/dumb-init", "--"]
CMD [ "/app" ]


