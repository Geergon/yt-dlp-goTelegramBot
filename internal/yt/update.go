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
	log.Println("Перевірка оновлення gallery-dl через apk")
	var output strings.Builder

	cmdUpdate := exec.Command("apk", "update")
	updateOutput, err := cmdUpdate.CombinedOutput()
	output.WriteString("Оновлення кешу apk:\n")
	output.WriteString(string(updateOutput))
	if err != nil {
		log.Printf("apk update error: %v\nOutput: %s", err, string(updateOutput))
		output.WriteString("\nПомилка оновлення кешу: " + err.Error())
		return output.String()
	}
	log.Println("apk update:\n", string(updateOutput))

	cmdUpgrade := exec.Command("apk", "upgrade", "gallery-dl")
	upgradeOutput, err := cmdUpgrade.CombinedOutput()
	output.WriteString("\nОновлення gallery-dl:\n")
	output.WriteString(string(upgradeOutput))
	if err != nil {
		log.Printf("apk upgrade gallery-dl error: %v\nOutput: %s", err, string(upgradeOutput))
		output.WriteString("\nПомилка оновлення gallery-dl: " + err.Error())
	} else {
		log.Println("apk upgrade gallery-dl:\n", string(upgradeOutput))
	}

	cmdVersion := exec.Command("gallery-dl", "--version")
	versionOutput, err := cmdVersion.CombinedOutput()
	output.WriteString("\nПоточна версія gallery-dl:\n")
	output.WriteString(string(versionOutput))
	if err != nil {
		log.Printf("gallery-dl --version error: %v\nOutput: %s", err, string(versionOutput))
		output.WriteString("\nПомилка перевірки версії: " + err.Error())
	} else {
		log.Println("gallery-dl version:\n", string(versionOutput))
	}

	return output.String()
}
