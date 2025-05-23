package yt

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os/exec"
)

type VideoInfo struct {
	IsLive  bool   `json:"isLive"`
	WasLive bool   `json:"wasLive"`
	ID      string `json:"ID"`
	Title   string `json:"Title"`
}

func DownloadYTVideo(url string, info *VideoInfo) {
	cmd := exec.Command("yt-dlp",
		"-f", "bv[filesize<500M][ext=mp4]+ba[ext=m4a]/bv[height=720][filesize<400M][ext=mp4]+ba[ext=m4a]/w",
		"--merge-output-format", "mp4", "-o", "%(id)s.%(ext)s",
		url,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("yt-dlp error: %v\nOutput: %s", err, string(output))
	}

	log.Printf("Завантаження %s завершено успішно \n", url)
}

func DownloadTTVideo(videoURL string, info *VideoInfo) {
	resp, err := http.PostForm("http://youtube-dl:8080", url.Values{
		"url": {videoURL}, // або інша назва параметра, залежно від форми
	})
	if err != nil {
		log.Println("Помилка надсилання запиту до youtube-dl:", err)
	}
	defer resp.Body.Close()

	log.Println("Відповідь від youtube-dl:", resp.Status)
}

func DownloadInstaVideo(url string, info *VideoInfo) {
	cmd := exec.Command("yt-dlp",
		"-f", "mp4",
		"--no-playlist",
		"--output", "%(id)s.%(ext)s",
		url,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("yt-dlp error: %v\nOutput: %s", err, string(output))
	}
	log.Printf("Завантаження %s завершено успішно \n", url)
}

func GetThumb(url string, info *VideoInfo) string {
	cmd := exec.Command("yt-dlp",
		"--skip-download",
		"--write-thumbnail",
		"--convert-thumbnails", "jpg",
		"--output", "%(id)s.%(ext)s",
		url,
	)

	err := cmd.Run()
	if err != nil {
		log.Printf("Помилка при отриманні прев'ю: %v", err)
	}
	return fmt.Sprintf("/download/%s.jpg", info.ID)
}
