package yt

import (
	"fmt"
	"log"
	"os/exec"
)

type VideoInfo struct {
	IsLive  bool   `json:"isLive"`
	WasLive bool   `json:"wasLive"`
	ID      string `json:"ID"`
	Title   string `json:"Title"`
}

func DownloadYTVideo(url string, info *VideoInfo) error {
	cmd := exec.Command("yt-dlp",
		"-f", "bv[filesize<500M][ext=mp4]+ba[ext=m4a]/bv[height=720][filesize<400M][ext=mp4]+ba[ext=m4a]/w",
		"--merge-output-format", "mp4", "-o", "%(id)s.%(ext)s",
		url,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("yt-dlp error: %v\nOutput: %s", err, string(output))
	}

	log.Println("Завантаження завершено успішно")
	return nil
}

func DownloadTTVideo(url string, info *VideoInfo) {
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
}

func DownloadInstaVideo(url string, info *VideoInfo) (string, error) {
	videoName := fmt.Sprintf("%s.mp4", info.ID)
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
	return videoName, nil
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
	return fmt.Sprintf("%s.jpg", info.ID)
}
