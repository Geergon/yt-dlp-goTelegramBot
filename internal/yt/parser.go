package yt

import (
	"fmt"
	"io/fs"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
)

func IsUrl(str string) bool {
	u, err := url.Parse(str)
	return err == nil && u.Scheme != "" && u.Host != ""
}

func RemoveYouTubeListParam(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	q := u.Query()
	q.Del("list")
	q.Del("index")
	u.RawQuery = q.Encode()

	return u.String()
}

func GetYoutubeURL(text string) (string, bool) {
	ytUrlRegexp := regexp.MustCompile(`((?:https?:)?\/\/)?((?:www|m)\.)?((?:youtube\.com|music\.youtube\.com|youtu.be))(\/(?:watch\?v=|embed\/|v\/|playlist\?list=|album\/|channel\/)?)?([\w\-]+)([\S]*)?`)
	url := ytUrlRegexp.FindString(text)
	return url, url != ""
}

func GetTikTokURL(text string) (string, bool) {
	ttr := regexp.MustCompile(`(:?https?:\/\/)?(:?www\.|m\.|vm\.|vt\.)?tiktok\.com\/(:?(:?[a-zA-Z0-9._-]+\/?)|(:?@[a-zA-Z0-9._]+\/(:?photo|video)+\/\d+\?[\S]+))`)
	url := ttr.FindString(text)
	return url, url != ""
}

func GetInstaURL(text string) (string, bool) {
	ir := regexp.MustCompile(`https?:\/\/(www\.)?instagram\.com\/(reel|p|tv|stories)\/[A-Za-z0-9_\-\.]+\/?\??[\S]*`)
	url := ir.FindString(text)
	return url, url != ""
}

func GetVideoInfo(url string, platform Platform) (VideoInfo, error) {
	args := []string{
		"--skip-download",
		"--no-playlist",
		"--print", "%(duration|0)s %(is_live|False)s %(was_live|False)s",
	}

	if hasCookies(platform) {
		args = append(args, "--cookies", cookieFiles[platform])
	}

	args = append(args, "--", url)

	out, err := exec.Command("yt-dlp", args...).Output()
	if err != nil {
		return VideoInfo{}, fmt.Errorf("yt-dlp -j: %w", err)
	}

	fields := strings.Fields(string(out))
	if len(fields) < 3 {
		return VideoInfo{}, fmt.Errorf("failed to parse video duration: %w", err)
	}

	duration, err := strconv.Atoi(fields[0])
	if err != nil {
		return VideoInfo{}, fmt.Errorf("failed to parse video duration: %w", err)
	}
	isLive, err := strconv.ParseBool(fields[1])
	if err != nil {
		return VideoInfo{}, fmt.Errorf("failed to parse video info: %w", err)
	}
	wasLive, err := strconv.ParseBool(fields[2])
	if err != nil {
		return VideoInfo{}, fmt.Errorf("failed to parse video info: %w", err)
	}

	return VideoInfo{Duration: duration, IsLive: isLive, WasLive: wasLive}, nil
}

//	func GetVideoName(url string, info *VideoInfo) string {
//		videoName := fmt.Sprintf("%s.mp4", info.ID)
//		return videoName
//	}
func GetPhotoPathList(dir string) ([]string, bool, bool, bool, string) {
	photo := os.DirFS(dir)

	jpgFiles, err := fs.Glob(photo, "*.jpg")
	if err != nil {
		log.Printf("Помилка пошуку jpg в %s: %v", dir, err)
	}

	pngFiles, err := fs.Glob(photo, "*.png")
	if err != nil {
		log.Printf("Помилка пошуку png в %s: %v", dir, err)
	}

	mp3Files, err := fs.Glob(photo, "*.mp3")
	if err != nil {
		log.Printf("Помилка пошуку mp3 в %s: %v", dir, err)
	}

	mp4Files, err := fs.Glob(photo, "*.mp4")
	if err != nil {
		log.Printf("Помилка пошуку mp4 в %s: %v", dir, err)
	}

	hasMusic := false
	musicPath := ""
	if len(mp3Files) != 0 {
		hasMusic = true
		musicPath = path.Join(dir, mp3Files[0])
	}

	var isExist bool
	var isVideo bool

	var videoPath []string
	if len(mp4Files) != 0 {
		videoPath = append(videoPath, path.Join(dir, mp4Files[0]))
		isExist = true
		isVideo = true
		return videoPath, isExist, isVideo, hasMusic, musicPath
	}

	var photos []string
	for _, photo := range jpgFiles {
		photos = append(photos, path.Join(dir, photo))
	}
	for _, photo := range pngFiles {
		photos = append(photos, path.Join(dir, photo))
	}
	log.Println(photos)
	if len(photos) != 0 {
		isExist = true
		isVideo = false
		return photos, isExist, isVideo, hasMusic, musicPath
	}
	isExist = false
	isVideo = false

	return nil, isExist, isVideo, hasMusic, musicPath
}
