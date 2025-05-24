package yt

import (
	"log"
	"os/exec"
	"strings"
)

func UpdateYtdlp() string {
	ytdlpVersionOld := exec.Command("yt-dlp", "--version")
	outputVersionOld, err := ytdlpVersionOld.Output()
	if err != nil {
		log.Printf("Помилка при отриманні версії yt-dlp: %v\n Output: %s \n", err, string(outputVersionOld))
	}
	versionOld := strings.TrimSpace(string(outputVersionOld))
	log.Printf("Поточна версія yt-dlp: %s\n", versionOld)
	log.Println("Перевірка оновлення yt-dlp")
	cmd := exec.Command("yt-dlp", "-U")
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("yt-dlp error: %v\nOutput: %s \n", err, string(output))
	}

	ytdlpVersionNew := exec.Command("yt-dlp", "--version")
	outputVersionNew, err := ytdlpVersionNew.Output()
	if err != nil {
		log.Printf("Помилка при отриманні версії yt-dlp: %v\n Output: %s \n", err, string(outputVersionNew))
	}
	versionNew := strings.TrimSpace(string(outputVersionNew))

	if strings.Contains(string(output), "is up to date") || versionOld == versionNew {
		log.Printf("Оновлень немає, поточна версія yt-dlp: %s \n", string(outputVersionOld))
	} else {
		log.Printf("Оновлення завершено, поточна версія yt-dlp: %s \n", string(outputVersionNew))
	}
	return string(output)
}
