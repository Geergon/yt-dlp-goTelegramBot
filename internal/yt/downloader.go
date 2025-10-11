package yt

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path"
	"strings"
	"sync"

	"github.com/spf13/viper"
)

type DownloadRequest struct {
	URL string `json:"url"`
}
type VideoInfo struct {
	Duration int  `json:"duration"`
	IsLive   bool `json:"is_live"`
	WasLive  bool `json:"was_live"`
	// ID      string `json:"id"`
	// Title   string `json:"title"`
}

var viperMutex sync.RWMutex

func DownloadYTVideo(url string, output string, longVideoDownload bool) (bool, error) {
	viperMutex.RLock()
	filter := viper.GetString("yt-dlp_filter")
	duration := viper.GetString("duration")
	viperMutex.RUnlock()

	cookies := "./cookies/cookiesYT.txt"
	var useCookies bool
	if _, err := os.Stat(cookies); os.IsNotExist(err) {
		useCookies = false
	} else {
		useCookies = true
	}

	matchFilter := "!playlist"
	if !longVideoDownload {
		matchFilter = fmt.Sprintf("%s & duration<%s", matchFilter, duration)
	}

	args := []string{
		"--break-on-reject",
		"--match-filter", matchFilter,
		"-f", filter,
		"--merge-output-format", "mp4",
		"--output", output,
	}
	if useCookies {
		log.Println("Використовуємо кукі")
		args = append(args, "--cookies", cookies)
	}
	args = append(args, url)

	cmd := exec.Command("yt-dlp", args...)
	o, err := cmd.CombinedOutput()
	if err != nil {
		log.Println(err)
	}
	if err != nil {
		log.Printf("yt-dlp error (YouTube): %v\nOutput: %s", err, string(o))
		if strings.Contains(string(o), "rejected by filter") {
			return false, fmt.Errorf("URL %s є плейлистом, завантаження відхилено", url)
		}
		return false, err
	}

	log.Printf("Завантаження %s завершено успішно", url)
	return false, nil
}

func DownloadTTVideo(url string, output string) (bool, error) {
	cookies := "./cookies/cookiesTT.txt"

	var ytdlpErr error
	if _, err := os.Stat(cookies); os.IsNotExist(err) {
		ytdlpErr = runYtdlp(false, url, output, true, false)
	} else {
		ytdlpErr = runYtdlp(true, url, output, true, false)
	}

	if ytdlpErr == nil {
		if _, err := os.Stat(output); err == nil {
			return false, nil
		}
		log.Printf("yt-dlp succeeded but no %s for %s", output, url)
	}

	log.Printf("Намагаємось завантажити з gallery-dl TikTok URL %s через помилку yt-dlp : %v", url, ytdlpErr)
	var galleryErr error
	var isSuccess bool
	if _, err := os.Stat(cookies); os.IsNotExist(err) {
		isSuccess, galleryErr = runGalleryDl(false, url, true, false)
	} else {
		isSuccess, galleryErr = runGalleryDl(true, url, true, false)
	}
	if galleryErr != nil {
		return false, fmt.Errorf("gallery-dl failed after yt-dlp error: %w", galleryErr)
	}

	if isSuccess {
		return true, nil // Photo
	}

	if _, err := os.Stat(output); err == nil {
		return false, nil // Video
	}

	log.Printf("No valid output file found for %s", url)
	return false, os.ErrNotExist
}

func DownloadInstaVideo(url string, output string) (bool, error) {
	cookies := "./cookies/cookiesINSTA.txt"

	var ytdlpErr error
	if _, err := os.Stat(cookies); os.IsNotExist(err) {
		ytdlpErr = runYtdlp(false, url, output, false, true)
	} else {
		ytdlpErr = runYtdlp(true, url, output, false, true)
	}

	if ytdlpErr == nil {
		if _, err := os.Stat(output); err == nil {
			return false, nil
		}
		log.Printf("yt-dlp succeeded but no %s for %s", output, url)
	}

	log.Printf("Намагаємось завантажити з gallery-dl Instagram URL %s через помилку yt-dlp : %v", url, ytdlpErr)
	var galleryErr error
	var isSuccess bool
	if _, err := os.Stat(cookies); os.IsNotExist(err) {
		isSuccess, galleryErr = runGalleryDl(false, url, false, true)
	} else {
		isSuccess, galleryErr = runGalleryDl(true, url, false, true)
	}
	if galleryErr != nil {
		return false, fmt.Errorf("gallery-dl failed after yt-dlp error: %w", galleryErr)
	}

	if isSuccess {
		return true, nil // Photo
	}

	if _, err := os.Stat(output); err == nil {
		return false, nil // Video
	}

	log.Printf("No valid output file found for %s", url)
	return false, os.ErrNotExist
}

func DownloadAnyMedia(url string, output string) (bool, error) {
	var ytdlpErr error
	ytdlpErr = runYtdlp(false, url, output, false, false)

	if ytdlpErr == nil {
		if _, err := os.Stat(output); err == nil {
			return false, nil
		}
		log.Printf("yt-dlp succeeded but no %s for %s", output, url)
	}

	log.Printf("Намагаємось завантажити з gallery-dl URL %s через помилку yt-dlp : %v", url, ytdlpErr)
	var galleryErr error
	var isSuccess bool
	isSuccess, galleryErr = runGalleryDl(false, url, true, false)
	if galleryErr != nil {
		return false, fmt.Errorf("gallery-dl failed after yt-dlp error: %w", galleryErr)
	}

	if isSuccess {
		return true, nil // Photo
	}

	if _, err := os.Stat(output); err == nil {
		return false, nil // Video
	}

	log.Printf("No valid output file found for %s", url)
	return false, os.ErrNotExist
}

func GetThumb(url string, platform string) string {
	var cookies string
	switch platform {
	case "YouTube":
		cookies = "./cookies/cookiesYT.txt"
	case "TikTok":
		cookies = "./cookies/cookiesTT.txt"
	case "Instagram":
		cookies = "./cookies/cookiesINSTA.txt"
	}
	args := []string{
		"--skip-download",
		"--write-thumbnail",
		"--convert-thumbnails", "jpg",
		"--output", "thumb.%(ext)s",
	}
	if _, err := os.Stat(cookies); !os.IsNotExist(err) {
		log.Println("Використовуємо кукі")
		args = append(args, "--cookies", cookies)
	}
	args = append(args, url)

	cmd := exec.Command("yt-dlp", args...)

	err := cmd.Run()
	if err != nil {
		log.Printf("Помилка при отриманні прев'ю: %v", err)
		return ""
	}
	return "thumb.jpg"
}

func DownloadAudio(url string, platform string) ([]string, error) {
	dir := "./audio"
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err = os.Mkdir(dir, 0o755)
		if err != nil {
			log.Printf("Помилка створення папки %s: %v", dir, err)
			return nil, err
		}
	}

	audioDir := os.DirFS(dir)
	mp3Files, err := fs.Glob(audioDir, "*.mp3")
	if err != nil {
		fmt.Println("error")
	}
	for _, m := range mp3Files {
		path := path.Join(dir, m)
		os.Remove(path)
	}

	var cookies string
	switch platform {
	case "YouTube":
		cookies = "./cookies/cookiesYT.txt"
	case "TikTok":
		cookies = "./cookies/cookiesTT.txt"
	case "Instagram":
		cookies = "./cookies/cookiesINSTA.txt"
	}

	args := []string{
		"--extract-audio",
		"--embed-thumbnail",
		"--audio-format", "mp3",
		"--audio-quality", "192K",
		"-o", "./audio/%(title)s.%(ext)s",
	}

	if _, err := os.Stat(cookies); !os.IsNotExist(err) {
		log.Println("Використовуємо кукі")
		args = append(args, "--cookies", cookies)
	}

	args = append(args, url)

	cmd := exec.Command("yt-dlp", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("yt-dlp error (%s): %v\nOutput: %s", platform, err, string(output))
		return nil, err
	}
	log.Printf("yt-dlp download successful for %s", url)

	if err == nil {
		var audioName []string
		for _, audio := range mp3Files {
			audioName = append(audioName, audio)
		}
		if len(audioName) == 0 {
			log.Printf("Не знайдено MP3-файлів після завантаження для URL: %s", url)
			return nil, fmt.Errorf("не знайдено MP3-файлів після завантаження")
		}
		log.Printf("Знайдено аудіофайли: %v", audioName)
		return audioName, nil
	}
	return nil, err
}
