package yt

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path"
	"regexp"
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
	ttr := regexp.MustCompile(`((http(s)?:\/\/)?(www\.|m\.|vm\.|vt\.)?tiktok\.com\/((h5\/share\/usr\/|v\/|@[A-Za-z0-9_\-]+\/(video|photo)\/|embed\/|trending\?shareId=|share\/user\/)?[A-Za-z0-9_\-]+\/?)(?:\?[^ ]*)?)`)
	url := ttr.FindString(text)
	return url, url != ""
}

func GetInstaURL(text string) (string, bool) {
	ir := regexp.MustCompile(`https?:\/\/(www\.)?instagram\.com\/(reel|p|tv|stories)\/[A-Za-z0-9_\-\.]+\/?(\?[^ ]*)?(#[^ ]*)?`)
	url := ir.FindString(text)
	return url, url != ""
}

func GetVideoInfo(url string, platform string) (*VideoInfo, error) {
	var cookies string
	switch platform {
	case "YouTube":
		cookies = "./cookies/cookiesYT.txt"
	case "TikTok":
		cookies = "./cookies/cookiesTT.txt"
	case "Instagram":
		cookies = "./cookies/cookiesINSTA.txt"
	}

	if _, err := os.Stat(cookies); !os.IsNotExist(err) {
		cmd := exec.Command("sh", "-c", fmt.Sprintf(`yt-dlp --cookies "%s" -j "%s" | jq -c '{Duration: .duration}'`, cookies, url))
		out, err := cmd.Output()
		if err != nil {
			log.Printf("помилка отримання інформації про відео: %v", err)
			return nil, fmt.Errorf("помилка виконання команди: %v", err)
		}

		var info VideoInfo
		if err := json.Unmarshal(out, &info); err != nil {
			log.Printf("JSON: %s", out)
			return nil, fmt.Errorf("помилка парсингу JSON: %v", err)
		}

		return &info, nil

	} else {
		cmd := exec.Command("sh", "-c", fmt.Sprintf(`yt-dlp -j "%s" | jq -c '{is_live: .is_live, was_live: .was_live,duration: .duration}'`, url))
		out, err := cmd.Output()
		if err != nil {
			log.Printf("помилка отримання інформації про відео: %v", err)
			return nil, fmt.Errorf("помилка виконання команди: %v", err)
		}

		var info VideoInfo
		if err := json.Unmarshal(out, &info); err != nil {
			log.Printf("JSON: %s", out)
			return nil, fmt.Errorf("помилка парсингу JSON: %v", err)
		}

		return &info, nil

	}
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
