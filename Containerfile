FROM golang:1.24.3-alpine

WORKDIR /yt-dlp-goTelegramBot

COPY go.* ./ 

RUN go mod download

RUN apk --no-cache add jq
RUN apk --no-cache add yt-dlp
RUN apk --no-cache add curl

RUN curl -L https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp \
  -o /usr/local/bin/yt-dlp && chmod +x /usr/local/bin/yt-dlp

COPY . .

RUN go build -o main main.go

CMD [ "./main" ]
