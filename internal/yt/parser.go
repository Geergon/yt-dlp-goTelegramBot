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

func GetYoutubeURL(text string) (string, bool) {
	ytUrlRegexp := regexp.MustCompile(`((?:https?:)?\/\/)?((?:www|m)\.)?((?:youtube\.com|youtu.be))(\/(?:[\w\-]+\?v=|embed\/|v\/)?)([\w\-]+)(\S+)?`)
	url := ytUrlRegexp.FindString(text)
	return url, url != ""
}

func GetTikTokURL(text string) (string, bool) {
	ttr := regexp.MustCompile(`((http(s)?:\/\/)?(www\.|m\.|vm\.)?tiktok\.com\/((h5\/share\/usr\/|v\/|@[A-Za-z0-9_\-]+\/video\/|embed\/|trending\?shareId=|share\/user\/)?[A-Za-z0-9_\-]+\/?)(?:\?[^ ]*)?)`)
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
func GetPhotoPathList() ([]string, bool) {
	dir := "./photo"
	photo := os.DirFS(dir)
	jpgFiles, err := fs.Glob(photo, "*.jpg")
	if err != nil {
		fmt.Println("error")
	}

	mp3Files, err := fs.Glob(photo, "*.mp3")
	if err != nil {
		fmt.Println("error")
	}
	for _, m := range mp3Files {
		path := path.Join(dir, m)
		os.Remove(path)
	}

	var photos []string
	for _, photo := range jpgFiles {
		photos = append(photos, path.Join(dir, photo))
	}
	log.Println(photos)
	if len(photos) != 0 {
		return photos, true
	}
	return nil, false
}
