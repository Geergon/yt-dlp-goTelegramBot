package yt

import (
	"context"
	"fmt"
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

func DownloadYTVideo(ctx context.Context, url string, info *VideoInfo) error {
	formats := []string{
		"bv[filesize<500M][ext=mp4]+ba[ext=m4a]/bv[height=720][filesize<400M][ext=mp4]+ba[ext=m4a]/w",
		"bv[height=480][filesize<300M][ext=mp4]+ba[ext=m4a]/w", // Менша якість для повторної спроби
	}

	cmd := exec.CommandContext(ctx,
		"yt-dlp",
		"-f", formats[0],
		"--merge-output-format", "mp4",
		"-o", "%(id)s.%(ext)s",
		url,
	)
	if ctx.Value("attempt") != nil && ctx.Value("attempt").(int) > 1 {
		cmd.Args[2] = "-f"
		cmd.Args[3] = formats[1]
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("yt-dlp error (YouTube): %v\nOutput: %s", err, string(output))
		return err
	}

	log.Printf("Завантаження %s завершено успішно", url)
	return nil
}

func DownloadTTVideo(ctx context.Context, url string, info *VideoInfo) error {
	cmd := exec.CommandContext(ctx,
		"yt-dlp",
		"-f", "mp4",
		"--no-playlist",
		"--output", "%(id)s.%(ext)s",
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

func DownloadInstaVideo(ctx context.Context, url string, info *VideoInfo) error {
	cmd := exec.CommandContext(ctx,
		"yt-dlp",
		"-f", "mp4",
		"--no-playlist",
		"--output", "%(id)s.%(ext)s",
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
