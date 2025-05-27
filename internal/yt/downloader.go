package yt

import (
	"fmt"
	"log"
	"os"
	"os/exec"
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

func DownloadTTVideo(url string) (bool, error) {
	cookies := "./cookies/cookiesTT.txt"

	var ytdlpErr error
	if _, err := os.Stat(cookies); os.IsNotExist(err) {
		ytdlpErr = runYtdlp(false, url, true, false)
	} else {
		ytdlpErr = runYtdlp(true, url, true, false)
	}

	if ytdlpErr == nil {
		if _, err := os.Stat("output.mp4"); err == nil {
			return false, nil
		}
		log.Printf("yt-dlp succeeded but no output.mp4 for %s", url)
	}

	log.Printf("Намагаємось завантажити з gallery-dl TikTok URL %s через помилку yt-dlp : %v", url, ytdlpErr)
	var galleryErr error
	if _, err := os.Stat(cookies); os.IsNotExist(err) {
		galleryErr = runGalleryDl(false, url, true, false)
	} else {
		galleryErr = runGalleryDl(true, url, true, false)
	}
	if galleryErr != nil {
		return false, fmt.Errorf("gallery-dl failed after yt-dlp error: %w", galleryErr)
	}

	photoExts := []string{".jpg", ".jpeg", ".png"}
	for _, ext := range photoExts {
		if _, err := os.Stat("output" + ext); err == nil {
			if ext != ".jpg" {
				if err := os.Rename("output"+ext, "output.jpg"); err != nil {
					log.Printf("Failed to rename output%s to output.jpg: %v", ext, err)
				}
			}
			return true, nil // Photo
		}
	}

	if _, err := os.Stat("output.mp4"); err == nil {
		return false, nil // Video
	}

	log.Printf("No valid output file found for %s", url)
	return false, os.ErrNotExist
}

func DownloadInstaVideo(url string) (bool, error) {
	cookies := "./cookies/cookiesINSTA.txt"

	var ytdlpErr error
	if _, err := os.Stat(cookies); os.IsNotExist(err) {
		ytdlpErr = runYtdlp(false, url, false, true)
	} else {
		ytdlpErr = runYtdlp(true, url, false, true)
	}

	if ytdlpErr == nil {
		if _, err := os.Stat("output.mp4"); err == nil {
			return false, nil
		}
		log.Printf("yt-dlp succeeded but no output.mp4 for %s", url)
	}

	log.Printf("Намагаємось завантажити з gallery-dl Instagram URL %s через помилку yt-dlp : %v", url, ytdlpErr)
	var galleryErr error
	if _, err := os.Stat(cookies); os.IsNotExist(err) {
		galleryErr = runGalleryDl(false, url, true, false)
	} else {
		galleryErr = runGalleryDl(true, url, true, false)
	}
	if galleryErr != nil {
		return false, fmt.Errorf("gallery-dl failed after yt-dlp error: %w", galleryErr)
	}

	photoExts := []string{".jpg", ".jpeg", ".png"}
	for _, ext := range photoExts {
		if _, err := os.Stat("output" + ext); err == nil {
			if ext != ".jpg" {
				if err := os.Rename("output"+ext, "output.jpg"); err != nil {
					log.Printf("Failed to rename output%s to output.jpg: %v", ext, err)
				}
			}
			return true, nil // Photo
		}
	}

	if _, err := os.Stat("output.mp4"); err == nil {
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
	if _, err := os.Stat(cookies); os.IsNotExist(err) {
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
	} else {
		cmd := exec.Command("yt-dlp",
			"--skip-download",
			"--write-thumbnail",
			"--convert-thumbnails", "jpg",
			"--cookies", cookies,
			"--output", "thumb.%(ext)s",
			url,
		)

		err := cmd.Run()
		if err != nil {
			log.Printf("Помилка при отриманні прев'ю: %v", err)
		}
		return "thumb.jpg"
	}
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

func runGalleryDl(useCookies bool, url string, isTT bool, isInsta bool) error {
	cookiesTT := "./cookies/cookiesTT.txt"
	cookiesINSTA := "./cookies/cookiesINSTA.txt"
	var platform string
	var cookies string
	if isTT {
		platform = "TikTok"
		cookies = cookiesTT
	}
	if isInsta {
		platform = "Instagram"
		cookies = cookiesINSTA
	}
	if useCookies {
		cmd := exec.Command(
			"gallery-dl",
			"-o", "overwrite=true",
			"--no-part",
			"-D", ".",
			"--cookies", cookies,
			"-f", "output.{extension}",
			"-o", "directory=",
			url,
		)
		output, err := cmd.CombinedOutput()
		if err != nil {
			log.Printf("gallery-dl error (%s): %v\nOutput: %s", platform, err, string(output))
			return err
		}
		log.Printf("gallery-dl download successful for %s", url)
		return nil
	} else {
		cmd := exec.Command(
			"gallery-dl",
			"-o", "overwrite=true",
			"--no-part",
			"-D", ".",
			"-f", "output.{extension}",
			"-o", "directory=",
			url,
		)
		output, err := cmd.CombinedOutput()
		if err != nil {
			log.Printf("gallery-dl error (%s): %v\nOutput: %s", platform, err, string(output))
			return err
		}
		log.Printf("gallery-dl download successful for %s", url)
		return nil
	}
}
