package yt

import (
	"log"
	"os/exec"
	"strings"
)

func UpdateYtdlp() string {
	log.Println("Перевірка оновлення yt-dlp")
	cmd := exec.Command("yt-dlp", "-U")
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("yt-dlp error: %v\nOutput: %s \n", err, string(output))
	}
	log.Println(string(output))
	return string(output)
}

func UpdateGallerydl() string {
	log.Println("Перевірка оновлення gallery-dl через pip")
	var output strings.Builder

	cmdVersion := exec.Command("gallery-dl", "--version")
	versionOutput, err := cmdVersion.CombinedOutput()
	// output.WriteString("\nПоточна версія gallery-dl:\n")
	oldVersion := string(versionOutput)
	// output.WriteString(oldVersion)
	if err != nil {
		log.Printf("gallery-dl --version error: %v\nOutput: %s", err, string(versionOutput))
		output.WriteString("\nПомилка перевірки версії: " + err.Error())
	} else {
		log.Println("gallery-dl version:\n", string(versionOutput))
	}

	cmdUpgrade := exec.Command("pip3", "install", "--upgrade", "--break-system-packages", "gallery-dl")
	upgradeOutput, err := cmdUpgrade.CombinedOutput()
	output.WriteString("Оновлення gallery-dl:\n")
	// output.WriteString(string(upgradeOutput))
	if err != nil {
		log.Printf("pip3 upgrade gallery-dl error: %v\nOutput: %s", err, string(upgradeOutput))
		output.WriteString("\nПомилка оновлення gallery-dl: " + err.Error())
		return output.String()
	}
	log.Println("pip3 upgrade gallery-dl:\n", string(upgradeOutput))

	cmdVersion = exec.Command("gallery-dl", "--version")
	versionOutput, err = cmdVersion.CombinedOutput()
	// output.WriteString("\nПоточна версія gallery-dl:\n")
	newVersion := string(versionOutput)
	// output.WriteString(newVersion)
	if err != nil {
		log.Printf("gallery-dl --version error: %v\nOutput: %s", err, string(versionOutput))
		output.WriteString("\nПомилка перевірки версії: " + err.Error())
	} else {
		log.Println("gallery-dl version:\n", string(versionOutput))
	}
	if oldVersion == newVersion {
		output.WriteString("\ngallery-dl вже оновлено до останньої версії: ")
		output.WriteString(newVersion)
	} else {
		output.WriteString("\ngallery-dl оновлено до останньої версії: ")
		output.WriteString(newVersion)
	}

	return output.String()
}
