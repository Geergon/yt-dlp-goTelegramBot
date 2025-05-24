package yt

import (
	"fmt"
	"log"
	"os/exec"
)

type DownloadRequest struct {
	URL string `json:"url"`
}
type VideoInfo struct {
	IsLive  bool   `json:"isLive"`
	WasLive bool   `json:"wasLive"`
	ID      string `json:"ID"`
	Title   string `json:"Title"`
}

func DownloadYTVideo(url string, info *VideoInfo) error {
	cmd := exec.Command("yt-dlp",
		"-f", "bv[filesize<500M][ext=mp4]+ba[ext=m4a]/bv[height=720][filesize<400M][ext=mp4]+ba[ext=m4a]/w",
		"--merge-output-format", "mp4", "-o", "%(id)s.%(ext)s",
		url,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("yt-dlp error: %v\nOutput: %s", err, string(output))
		return err
	}

	log.Printf("Завантаження %s завершено успішно \n", url)
	return nil
	// url := "http://ytdl_material:17442/api/downloadFile?apiKey=f45a3732-5346-4408-8148-b91d24945b01"
	// p := fmt.Sprintf(`{
	// 	"url": "%s",
	// 	"customArgs": "-f bv[filesize<500M][ext=mp4]+ba[ext=m4a]/bv[height=720][filesize<400M][ext=mp4]+ba[ext=m4a]/w"
	// 	}`, videoURL)
	// payload := strings.NewReader(p)
	//
	// req, _ := http.NewRequest("POST", url, payload)
	//
	// req.Header.Add("Content-Type", "application/json")
	// req.Header.Add("Accept", "application/json")
	//
	// res, err := http.DefaultClient.Do(req)
	// if err != nil {
	// 	log.Printf("HTTP помилка: %v", err)
	// 	return
	// }
	// defer res.Body.Close()
	// body, _ := io.ReadAll(res.Body)
	//
	// fmt.Println(res)
	// fmt.Println(string(body))
	// requestBody := DownloadRequest{
	// 	URL:        videoURL,
	// }
	//
	// jsonData, err := json.Marshal(requestBody)
	// if err != nil {
	// 	log.Printf("Помилка при серіалізації JSON: %v", err)
	// 	return
	// }
	//
	// req, err := http.NewRequest("POST", fmt.Sprintf("%s/api/downloadFile?apiKey=%s", apiURL, apiKey), bytes.NewBuffer(jsonData))
	// if err != nil {
	// 	log.Printf("Помилка при створенні запиту: %v", err)
	// 	return
	// }
	//
	// req.Header.Set("Content-Type", "application/json")
	// req.Header.Set("Accept", "application/json")
	//
	// client := &http.Client{}
	// res, err := client.Do(req)
	// if err != nil {
	// 	log.Printf("Помилка при виконанні запиту: %v", err)
	// 	return
	// }
	// defer res.Body.Close()
	//
	// body, err := io.ReadAll(res.Body)
	// if err != nil {
	// 	log.Printf("Помилка при читанні відповіді: %v", err)
	// 	return
	// }
	//
	// fmt.Printf("Статус: %s\n", res.Status)
	// fmt.Printf("Відповідь: %s\n", string(body))
	// log.Printf("Завантаження %s завершено успішно \n", videoURL)
}

func DownloadTTVideo(url string, info *VideoInfo) {
	cmd := exec.Command("yt-dlp",
		"-f", "mp4",
		"--no-playlist",
		"--output", "%(id)s.%(ext)s",
		url,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("yt-dlp error: %v\nOutput: %s", err, string(output))
	}
	log.Printf("Завантаження %s завершено успішно \n", url)
}

func DownloadInstaVideo(url string, info *VideoInfo) {
	cmd := exec.Command("yt-dlp",
		"-f", "mp4",
		"--no-playlist",
		"--output", "%(id)s.%(ext)s",
		url,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("yt-dlp error: %v\nOutput: %s", err, string(output))
	}
	log.Printf("Завантаження %s завершено успішно \n", url)
}

func GetThumb(url string, info *VideoInfo) string {
	cmd := exec.Command("yt-dlp",
		"--skip-download",
		"--write-thumbnail",
		"--convert-thumbnails", "jpg",
		"--output", "%(id)s.%(ext)s",
		url,
	)

	err := cmd.Run()
	if err != nil {
		log.Printf("Помилка при отриманні прев'ю: %v", err)
	}
	return fmt.Sprintf("%s.jpg", info.ID)
}
