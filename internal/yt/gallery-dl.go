package yt

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

func runGalleryDl(useCookies bool, url string, isTT bool, isInsta bool) (bool, error) {
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

	for _, ext := range []string{"jpg", "png", "jpeg"} {
		matches, _ := filepath.Glob(fmt.Sprintf("output-*.%s", ext))
		for _, match := range matches {
			if err := os.Remove(match); err != nil {
				log.Printf("Failed to remove %s: %v", match, err)
			}
		}
		if err := os.Remove(fmt.Sprintf("output.%s", ext)); err != nil && !os.IsNotExist(err) {
			log.Printf("Failed to remove output.%s: %v", ext, err)
		}
	}

	args := []string{
		"-o", "overwrite=true",
		"--no-part",
		"-D", "photo",
		"-f", "output-{num:02d}.{extension}",
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
		return false, err
	}
	log.Printf("gallery-dl download successful for %s", url)
	return true, nil
}
