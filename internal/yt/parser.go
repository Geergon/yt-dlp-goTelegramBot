package yt

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os/exec"
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
	ttr := regexp.MustCompile(`((http(s)?:\/\/)?(www\.|m\.|vm\.)?tiktok\.com\/((h5\/share\/usr\/|v\/|@[A-Za-z0-9_\-]+\/video\/|embed\/|trending\?shareId=|share\/user\/)?[A-Za-z0-9_\-]+\/?))`)
	url := ttr.FindString(text)
	return url, url != ""
}

func GetInstaURL(text string) (string, bool) {
	ir := regexp.MustCompile(`https?:\/\/(www\.)?instagram\.com\/(reel|p|tv|stories)\/[A-Za-z0-9_\-\.]+`)
	url := ir.FindString(text)
	return url, url != ""
}

func GetVideoInfo(url string) (*VideoInfo, error) {
	cmd := exec.Command("sh", "-c", fmt.Sprintf(`yt-dlp -j "%s" | jq -c '{ID: .id, Title: .fulltitle}'`, url))
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("помилка виконання команди: %v", err)
	}

	var info VideoInfo
	if err := json.Unmarshal(out, &info); err != nil {
		log.Printf("JSON: %s", out)
		return nil, fmt.Errorf("помилка парсингу JSON: %v", err)
	}

	return &info, nil
}

func GetVideoName(url string, info *VideoInfo) string {
	videoName := fmt.Sprintf("%s.mp4", info.ID)
	return videoName
}
