package yt

import (
	"log"
	"os/exec"
	"strings"
)

func runYtdlp(useCookies bool, url string, output string, platform Platform) error {
	cookies := cookieFiles[platform]

	args := []string{
		// "-f", "mp4",
		"--no-playlist",
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
