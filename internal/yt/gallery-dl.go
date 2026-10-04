package yt

import (
	"log"
	"os/exec"
)

func runGalleryDl(useCookies bool, url string, platform Platform) (string, error) {
	cookies := cookieFiles[platform]

	dir, err := createTempDir("gallery-dl-download-")
	if err != nil {
		return "", err
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
		return "", err
	}
	log.Printf("gallery-dl download successful for %s", url)
	return dir, nil
}
