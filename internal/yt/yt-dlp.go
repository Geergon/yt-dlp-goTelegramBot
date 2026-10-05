package yt

import (
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
)

func runYtdlp(useCookies bool, url string, output string, platform Platform) error {
	cookies := cookieFiles[platform]

	dir := filepath.Base(output)
	thumbPath := filepath.Join(dir, "thumb", "thumb.jpg")

	args := []string{
		// "-f", "mp4",
		"--no-playlist",
		"--write-thumbnail",
		"--convert-thumbnails", "jpg",
		"-o", "thumbnail:" + thumbPath,
		"--js-runtimes", "node",
		"--output", output,
	}
	if platform == TikTok {
		args = append(args, "-S", "vcodec:avc")
		args = append(args, "--referer", "https://example.com")
	}
	if useCookies {
		args = append(args, "--cookies", cookies)
	}
	args = append(args, url)

	cmd := exec.Command("yt-dlp", args...)
	o, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("yt-dlp error (%s): %v\nOutput: %s", platform, err, string(o))
		return err
	}
	log.Printf("yt-dlp download successful for %s", url)

	return nil
}

func HasAudioTrack(filePath string) bool {
	log.Println("Перевірка наявності звуку в відео...")

	cmd := exec.Command("ffprobe",
		"-v", "quiet",
		"-select_streams", "a",
		"-show_entries", "stream=codec_type",
		"-of", "csv=p=0",
		filePath,
	)
	out, err := cmd.Output()
	if err != nil {
		return false
	}

	if strings.TrimSpace(string(out)) == "audio" {
		log.Println("Відео має звук")
	}
	return strings.TrimSpace(string(out)) == "audio"
}

func DownloadFragment(url, fragment string) (DownloadResult, error) {
	viperMutex.RLock()
	filter := viper.GetString("yt-dlp_filter")
	viperMutex.RUnlock()

	dir, tempDirErr := createTempDir("media-download-")
	if tempDirErr != nil {
		return DownloadResult{}, tempDirErr
	}

	fileName := fmt.Sprintf("outputFrag%d.mp4", time.Now().UnixMilli())
	outputFile := filepath.Join(dir, fileName)

	cmd := exec.Command(
		"yt-dlp",
		"--download-sections", fmt.Sprintf("*%s", fragment),
		"-f", filter,
		"-o", outputFile,
		url,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("yt-dlp error: %v\nOutput: %s", err, string(output))
		return DownloadResult{}, err
	}

	files := listMedia(dir)
	if len(files) == 0 {
		return DownloadResult{}, fmt.Errorf("в директорії завантаження фрагменту пусто")
	}

	return DownloadResult{FilePath: files[0], MediaDir: dir, IsPhoto: false}, nil
}
