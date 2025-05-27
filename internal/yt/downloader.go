package yt

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/spf13/viper"
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

var viperMutex sync.RWMutex

func DownloadYTVideo(url string) error {
	viperMutex.RLock()
	filter := viper.GetString("yt-dlp_filter")
	viperMutex.RUnlock()

	cookies := "./cookies/cookiesYT.txt"
	if _, err := os.Stat(cookies); os.IsNotExist(err) {
		log.Println("Файл кукі не знайдено")
		cmd := exec.Command(
			"yt-dlp",
			"-f", filter,
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
	} else {
		log.Println("Файл кукі знайдено")
		cmd := exec.Command(
			"yt-dlp",
			"-f", filter,
			"--cookies", cookies,
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
}

func DownloadTTVideo(url string) (bool, []string, error) {
	cookies := "./cookies/cookiesTT.txt"

	var ytdlpErr error
	if _, err := os.Stat(cookies); os.IsNotExist(err) {
		ytdlpErr = runYtdlp(false, url, true, false)
	} else {
		ytdlpErr = runYtdlp(true, url, true, false)
	}

	if ytdlpErr == nil {
		if _, err := os.Stat("output.mp4"); err == nil {
			return false, nil, nil
		}
		log.Printf("yt-dlp succeeded but no output.mp4 for %s", url)
	}

	log.Printf("Намагаємось завантажити з gallery-dl TikTok URL %s через помилку yt-dlp : %v", url, ytdlpErr)
	var filePaths []string
	var galleryErr error
	if _, err := os.Stat(cookies); os.IsNotExist(err) {
		filePaths, galleryErr = runGalleryDl(false, url, true, false)
	} else {
		filePaths, galleryErr = runGalleryDl(true, url, true, false)
	}
	if galleryErr != nil {
		return false, nil, fmt.Errorf("gallery-dl failed after yt-dlp error: %w", galleryErr)
	}

	if len(filePaths) > 0 {
		return true, filePaths, nil // Photo
	}

	photoExts := []string{".jpg", ".jpeg", ".png"}
	for _, ext := range photoExts {
		if _, err := os.Stat("output" + ext); err == nil {
			if ext != ".jpg" {
				if err := os.Rename("output"+ext, "output.jpg"); err != nil {
					log.Printf("Failed to rename output%s to output.jpg: %v", ext, err)
				}
			}
			return true, filePaths, nil // Photo
		}
	}

	if _, err := os.Stat("output.mp4"); err == nil {
		return false, nil, nil // Video
	}

	log.Printf("No valid output file found for %s", url)
	return false, nil, os.ErrNotExist
}

func DownloadInstaVideo(url string) (bool, []string, error) {
	cookies := "./cookies/cookiesINSTA.txt"

	var ytdlpErr error
	if _, err := os.Stat(cookies); os.IsNotExist(err) {
		ytdlpErr = runYtdlp(false, url, false, true)
	} else {
		ytdlpErr = runYtdlp(true, url, false, true)
	}

	if ytdlpErr == nil {
		if _, err := os.Stat("output.mp4"); err == nil {
			return false, nil, nil
		}
		log.Printf("yt-dlp succeeded but no output.mp4 for %s", url)
	}

	log.Printf("Намагаємось завантажити з gallery-dl Instagram URL %s через помилку yt-dlp : %v", url, ytdlpErr)
	var filePaths []string
	var galleryErr error
	if _, err := os.Stat(cookies); os.IsNotExist(err) {
		filePaths, galleryErr = runGalleryDl(false, url, true, false)
	} else {
		filePaths, galleryErr = runGalleryDl(true, url, true, false)
	}
	if galleryErr != nil {
		return false, nil, fmt.Errorf("gallery-dl failed after yt-dlp error: %w", galleryErr)
	}

	if len(filePaths) > 0 {
		return true, filePaths, nil // Photo
	}

	photoExts := []string{".jpg", ".jpeg", ".png"}
	for _, ext := range photoExts {
		if _, err := os.Stat("output" + ext); err == nil {
			if ext != ".jpg" {
				if err := os.Rename("output"+ext, "output.jpg"); err != nil {
					log.Printf("Failed to rename output%s to output.jpg: %v", ext, err)
				}
			}
			return true, filePaths, nil // Photo
		}
	}

	if _, err := os.Stat("output.mp4"); err == nil {
		return false, nil, nil // Video
	}

	log.Printf("No valid output file found for %s", url)
	return false, nil, os.ErrNotExist
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

func runYtdlp(useCookies bool, url string, isTT bool, isInsta bool) error {
	cookiesTT := "./cookies/cookiesTT.txt"
	cookiesINSTA := "./cookies/cookiesINSTA.txt"
	outputFile := "output.%(ext)s"
	var platforma string
	var cookies string
	if isTT {
		platforma = "TikTok"
		cookies = cookiesTT
	}
	if isInsta {
		platforma = "Instagram"
		cookies = cookiesINSTA
	}
	args := []string{
		"-f", "mp4",
		"--no-playlist",
		"--output", outputFile,
	}
	if useCookies {
		args = append(args, "--cookies", cookies)
	}
	args = append(args, url)

	cmd := exec.Command("yt-dlp", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("yt-dlp error (%s): %v\nOutput: %s", platforma, err, string(output))
		return err
	}
	log.Printf("yt-dlp download successful for %s", url)
	return nil
}

func runGalleryDl(useCookies bool, url string, isTT bool, isInsta bool) (filePaths []string, err error) {
	var platform, cookies string
	switch {
	case isTT:
		platform = "TikTok"
		cookies = "./cookies/cookiesTT.txt"
	case isInsta:
		platform = "Instagram"
		cookies = "./cookies/cookiesINSTA.txt"
	default:
		return nil, fmt.Errorf("no platform specified (TikTok or Instagram)")
	}

	for _, ext := range []string{"jpg", "png", "jpeg"} {
		matches, _ := filepath.Glob(fmt.Sprintf("output-*.%s", ext))
		for _, match := range matches {
			if err := os.Remove(match); err != nil {
				log.Printf("Failed to remove %s: %v", match, err)
			}
		}
		if err := os.Remove(fmt.Sprintf("output.%s", ext)); err != nil && !os.IsNotExist(err) {
			log.Printf("Failed to remove output.%s: %v", ext, err)
		}
	}

	args := []string{
		"-o", "overwrite=false",
		"--no-part",
		"-D", ".",
		"-f", "output-{num:02d}.{extension}",
		"-o", "directory=",
	}
	if useCookies {
		args = append(args, "--cookies", cookies)
	}
	args = append(args, url)

	cmd := exec.Command("gallery-dl", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		log.Printf("gallery-dl error (%s): %v\nStderr: %s", platform, err, stderr.String())
		return nil, fmt.Errorf("gallery-dl failed for %s: %v", platform, err)
	}
	log.Printf("gallery-dl stdout (%s): %s", platform, stdout.String())
	log.Printf("gallery-dl download successful for %s", url)

	lines := strings.Split(stdout.String(), "\n")
	validExtensions := map[string]bool{
		".jpg":  true,
		".png":  true,
		".jpeg": true,
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		ext := strings.ToLower(filepath.Ext(line))
		if !validExtensions[ext] {
			log.Printf("Skipping file with invalid extension: %s", line)
			continue
		}
		if _, err := os.Stat(line); err != nil {
			log.Printf("File does not exist: %s", line)
			continue
		}
		filePaths = append(filePaths, line)
	}

	if len(filePaths) == 0 {
		return nil, fmt.Errorf("no valid image files found for %s", platform)
	}

	return filePaths, nil
}
