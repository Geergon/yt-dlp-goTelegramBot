package yt

import (
	"log"
	"os/exec"
	"strings"
	"time"
)

var lastUpdate time.Time

func ScheduleYtdlpUpdate() {
	go func() {
		for {
			now := time.Now()
			next := time.Date(now.Year(), now.Month(), now.Day(), 3, 0, 0, 0, now.Location())
			if now.After(next) || now.Equal(next) {
				next = next.Add(24 * time.Hour)
			}
			duration := next.Sub(now)

			time.Sleep(duration)

			if now.Truncate(24*time.Hour) != lastUpdate.Truncate(24*time.Hour) {
				UpdateYtdlp()
				lastUpdate = time.Now()
			}
		}
	}()
}

func UpdateYtdlp() {
	ytdlpVersionOld := exec.Command("yt-dlp", "--version")
	outputVersionOld, err := ytdlpVersionOld.Output()
	if err != nil {
		log.Printf("Помилка при отриманні версії yt-dlp: %v\n Output: %s \n", err, string(outputVersionOld))
		return
	}
	versionOld := strings.TrimSpace(string(outputVersionOld))
	log.Printf("Поточна версія yt-dlp: %s\n", versionOld)
	log.Println("Перевірка оновлення yt-dlp")
	cmd := exec.Command("yt-dlp", "-U")
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("yt-dlp error: %v\nOutput: %s \n", err, string(output))
		return
	}

	ytdlpVersionNew := exec.Command("yt-dlp", "--version")
	outputVersionNew, err := ytdlpVersionNew.Output()
	if err != nil {
		log.Printf("Помилка при отриманні версії yt-dlp: %v\n Output: %s \n", err, string(outputVersionNew))
		return
	}
	versionNew := strings.TrimSpace(string(outputVersionNew))

	if strings.Contains(string(output), "is up to date") || versionOld == versionNew {
		log.Printf("Оновлень немає, поточна версія yt-dlp: %s \n", string(outputVersionOld))
	} else {
		log.Printf("Оновлення завершено, поточна версія yt-dlp: %s \n", string(outputVersionNew))
	}
}
