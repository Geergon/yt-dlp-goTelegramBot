FROM golang:1.24.3-alpine

WORKDIR /yt-dlp-goTelegramBot

COPY go.* ./ 

RUN go mod download

COPY . .

RUN go build -o main main.go

CMD [ "./main" ]
