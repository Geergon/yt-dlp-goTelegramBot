package yt

import (
	"log"
	"os/exec"
)

func runYtdlp(useCookies bool, url string, output string, isTT bool, isInsta bool) error {
	cookiesTT := "./cookies/cookiesTT.txt"
	cookiesINSTA := "./cookies/cookiesINSTA.txt"
	// ext := "%(ext)s"
	// outputFile := fmt.Sprintf("%s.%s", output, ext)
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
		// "-f", "mp4",
		"--no-playlist",
		"--output", output,
	}
	if useCookies {
		args = append(args, "--cookies", cookies)
	}
	args = append(args, url)

	cmd := exec.Command("yt-dlp", args...)
	o, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("yt-dlp error (%s): %v\nOutput: %s", platforma, err, string(o))
		return err
	}
	log.Printf("yt-dlp download successful for %s", url)
	return nil
}
