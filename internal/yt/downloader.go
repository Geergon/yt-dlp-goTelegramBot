package yt

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
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

type DownloadResult struct {
	MediaDir      string
	FilePath      string
	ThumbnailPath string
	IsPhoto       bool
}

type Platform int

const (
	Unknown Platform = iota
	YouTube
	TikTok
	Instagram
)

func (p Platform) String() string {
	switch p {
	case Unknown:
		return "Unknown"
	case YouTube:
		return "YouTube"
	case TikTok:
		return "TikTok"
	case Instagram:
		return "Instagram"
	default:
		return ""
	}
}

var cookieFiles = map[Platform]string{
	TikTok:    "./cookies/cookiesTT.txt",
	Instagram: "./cookies/cookiesINSTA.txt",
	YouTube:   "./cookies/cookiesYT.txt",
}

var viperMutex sync.RWMutex

func DownloadMedia(url string, platform Platform) (DownloadResult, error) {
	switch platform {
	case YouTube:
		return downloadYTVideo(url)
	default:
		return downloadAnyMedia(url, platform)
	}
}

func downloadYTVideo(url string) (DownloadResult, error) {
	viperMutex.RLock()
	filter := viper.GetString("yt-dlp_filter")
	viperMutex.RUnlock()

	dir, tempDirErr := createTempDir("media-download-")
	if tempDirErr != nil {
		return DownloadResult{}, tempDirErr
	}

	cookies := "./cookies/cookiesYT.txt"
	var useCookies bool
	if _, err := os.Stat(cookies); os.IsNotExist(err) {
		useCookies = false
	} else {
		useCookies = true
	}

	// matchFilter := "!playlist"

	output := filepath.Join(dir, "%(title).100B.%(ext)s")
	thumbPath := filepath.Join(dir, "thumb", "thumb")

	args := []string{
		"--break-on-reject",
		// "--match-filter", matchFilter,
		"-f", filter,
		"--merge-output-format", "mp4",
		"--no-playlist",
		"--concurrent-fragments", "4",
		"--write-thumbnail",
		"--convert-thumbnails", "jpg",
		"-o", "thumbnail:" + thumbPath,
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
		log.Printf("yt-dlp error (YouTube): %v\nOutput: %s", err, string(o))
		if strings.Contains(string(o), "rejected by filter") {
			return DownloadResult{}, fmt.Errorf("URL %s є плейлистом, завантаження відхилено", url)
		}
		return DownloadResult{}, err
	}
	files := listMedia(dir)

	log.Printf("Завантаження %s завершено успішно", url)
	if len(files) == 0 {
		return DownloadResult{}, fmt.Errorf("в директорії завантаження %s відсутні файли", dir)
	}
	return DownloadResult{MediaDir: dir, FilePath: files[0], ThumbnailPath: thumbPath, IsPhoto: false}, nil
}

func downloadAnyMedia(url string, platform Platform) (DownloadResult, error) {
	dir, tempDirErr := createTempDir("media-download-")
	if tempDirErr != nil {
		return DownloadResult{}, tempDirErr
	}

	useCookies := hasCookies(platform)
	output := filepath.Join(dir, "%(id)s_%(autonumber)02d.%(ext)s")

	ytdlpErr := runYtdlp(useCookies, url, output, platform)

	if ytdlpErr == nil {
		if files := listMedia(dir); len(files) > 0 {
			return DownloadResult{MediaDir: dir, FilePath: files[0], IsPhoto: false}, nil
		}
		log.Printf("yt-dlp succeeded but no files in %s for %s", dir, url)
	}

	// yt-dlp failed, delete temp dir
	_ = os.RemoveAll(dir)

	log.Printf("Trying to download URL %s through gallery-dl (%s) due to yt-dlp error: %v", url, platform, ytdlpErr)
	galleryDir, galleryErr := runGalleryDl(useCookies, url, platform)
	if galleryErr != nil || len(listMedia(galleryDir)) == 0 {
		if galleryDir != "" {
			_ = os.RemoveAll(galleryDir)
		}
		return DownloadResult{}, fmt.Errorf("gallery-dl failed after yt-dlp error: %w", galleryErr)
	}

	files := listMedia(galleryDir)
	if len(files) == 0 {
		if galleryDir != "" {
			_ = os.RemoveAll(galleryDir)
		}
		return DownloadResult{}, fmt.Errorf("no media found for %s: %w", url, os.ErrNotExist)
	}

	return DownloadResult{MediaDir: galleryDir, IsPhoto: true}, nil // Photo and video through gallery-dl
}

func DownloadAudio(url string, platform Platform) ([]string, string, error) {
	dir, tempDirErr := createTempDir("audio-download-")
	if tempDirErr != nil {
		return nil, "", tempDirErr
	}

	audioDir := os.DirFS(dir)

	cookies := cookieFiles[platform]

	args := []string{
		"--extract-audio",
		"--embed-thumbnail",
		"--embed-metadata",
		"--audio-format", "mp3",
		"--audio-quality", "192K",
		"-o", path.Join(dir, "%(title)s.%(ext)s"),
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
		return nil, "", err
	}
	log.Printf("yt-dlp download successful for %s", url)

	newMp3Files, err := fs.Glob(audioDir, "*.mp3")
	if err != nil {
		log.Printf("Помилка при повторному отриманні списку файлів: %v", err)
		return nil, "", err
	}
	if len(newMp3Files) == 0 {
		log.Printf("Не знайдено MP3-файлів після завантаження для URL: %s", url)
		return nil, "", fmt.Errorf("не знайдено MP3-файлів після завантаження")
	}

	log.Printf("Знайдено аудіофайли: %v", newMp3Files)
	return newMp3Files, dir, nil
}

func createTempDir(name string) (dir string, ok error) {
	dir, err := os.MkdirTemp("", name)
	if err != nil {
		log.Printf("Помилка створення тимчасового каталогу: %v", err)
		return "", err
	}
	return dir, nil
}

func hasCookies(p Platform) bool {
	path, ok := cookieFiles[p]
	if !ok {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func listMedia(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() || strings.HasSuffix(e.Name(), ".part") {
			continue
		}
		files = append(files, filepath.Join(dir, e.Name()))
	}
	return files
}
