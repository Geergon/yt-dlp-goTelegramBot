package main

import (
	"log"
	"sync"

	"github.com/Geergon/yt-dlp-goTelegramBot/internal/tgbot"
	"github.com/celestix/gotgproto"
)

var (
	urlQueue       = make(chan tgbot.URLRequest, 100)
	processingURLs = sync.Map{}
	semaphore      = make(chan struct{}, 2)
)

func StartWorkers(client *gotgproto.Client, numWorkers int) {
	for i := range numWorkers {
		go func(workerID int) {
			for req := range urlQueue {
				// Захоплюємо семафор
				log.Printf("Черга URL: %v", urlQueue)
				semaphore <- struct{}{}
				log.Printf("Воркер %d обробляє URL: %s (команда: %s)", workerID, req.URL, req.Command)
				err := tgbot.ProcessURL(cacheDb, req)
				if err != nil {
					log.Printf("Помилка обробки URL %s: %v", req.URL, err)
					processingURLs.Delete(req.URL)
				}

				processingURLs.Delete(req.URL)
				// Звільняємо семафор
				<-semaphore
			}
		}(i)
	}
}
