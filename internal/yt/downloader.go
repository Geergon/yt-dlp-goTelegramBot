package yt

import (
	"log"
	"os/exec"
)

type DownloadRequest struct {
	URL string `json:"url"`
}
type VideoInfo struct {
	IsLive  bool   `json:"isLive"`
	WasLive bool   `json:"wasLive"`
	ID      string `json:"ID"`
	Title   string `json:"Title"`
}

func DownloadYTVideo(url string) error {
	cmd := exec.Command(
		"yt-dlp",
		"--cookies", "./cookies/cookiesYt.txt",
		"-f", "bv[filesize<500M][ext=mp4]+ba[ext=m4a]/bv[height=720][filesize<400M][ext=mp4]+ba[ext=m4a]/bv[height=480][filesize<300M][ext=mp4]+ba[ext=m4a]",
		"--merge-output-format", "mp4",
		"-o", "output.%(ext)s",
		url,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("yt-dlp error (YouTube): %v\nOutput: %s", err, string(output))
		return err
	}

	log.Printf("Завантаження %s завершено успішно", url)
	return nil
}

func DownloadTTVideo(url string) error {
	cmd := exec.Command(
		"yt-dlp",
		"-f", "mp4",
		"--no-playlist",
		"--output", "output.%(ext)s",
		url,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("yt-dlp error (TikTok): %v\nOutput: %s", err, string(output))
		return err
	}

	log.Printf("Завантаження %s завершено успішно", url)
	return nil
}

func DownloadInstaVideo(url string) error {
	cmd := exec.Command(
		"yt-dlp",
		"-f", "mp4",
		"--no-playlist",
		"--output", "output.%(ext)s",
		url,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("yt-dlp error (Instagram): %v\nOutput: %s", err, string(output))
		return err
	}

	log.Printf("Завантаження %s завершено успішно", url)
	return nil
}

func GetThumb(url string) string {
	cmd := exec.Command("yt-dlp",
		"--skip-download",
		"--write-thumbnail",
		"--convert-thumbnails", "jpg",
		"--output", "thumb.%(ext)s",
		url,
	)

	err := cmd.Run()
	if err != nil {
		log.Printf("Помилка при отриманні прев'ю: %v", err)
	}
	return "thumb.jpg"
}
