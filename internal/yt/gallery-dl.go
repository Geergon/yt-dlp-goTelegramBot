package yt

import (
	"log"
	"os"
	"os/exec"
)

func runGalleryDl(useCookies bool, url string, isTT bool, isInsta bool) (bool, string, error) {
	var platform string
	var cookies string
	if isTT {
		platform = "TikTok"
		cookies = "./cookies/cookiesTT.txt"
	}
	if isInsta {
		platform = "Instagram"
		cookies = "./cookies/cookiesINSTA.txt"
	}

	dir, err := os.MkdirTemp("", "gallery-dl-download-")
	if err != nil {
		log.Printf("Помилка створення тимчасового каталогу: %v", err)
		return false, "", err
	}

	args := []string{
		"-o", "overwrite=true",
		"--no-part",
		"-f", "{title}.{extension}",
		"-D", dir,
		"-o", "directory=",
	}
	if useCookies {
		args = append(args, "--cookies", cookies)
	}
	args = append(args, url)

	cmd := exec.Command("gallery-dl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("gallery-dl error (%s): %v\nOutput: %s", platform, err, string(output))
		return false, "", err
	}
	log.Printf("gallery-dl download successful for %s", url)
	return true, dir, nil
}
