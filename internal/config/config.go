package config

import (
	"log"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

var mu sync.RWMutex

func GetString(key string) string {
	mu.RLock()
	defer mu.RUnlock()
	return viper.GetString(key)
}

func GetBool(key string) bool {
	mu.RLock()
	defer mu.RUnlock()
	return viper.GetBool(key)
}

func GetInt(key string) int {
	mu.RLock()
	defer mu.RUnlock()
	return viper.GetInt(key)
}

func GetFloat64(key string) float64 {
	mu.RLock()
	defer mu.RUnlock()
	return viper.GetFloat64(key)
}

func GetIntSlice(key string) []int {
	mu.RLock()
	defer mu.RUnlock()
	return viper.GetIntSlice(key)
}

func Reload() {
	mu.Lock()
	defer mu.Unlock()
	if err := viper.ReadInConfig(); err != nil {
		log.Printf("Помилка перечитування конфігурації: %v", err)
		return
	}
}

func SetConfig() {
	mu.Lock()
	defer mu.Unlock()
	viper.SetConfigName("config")
	viper.SetConfigType("toml")
	viper.AddConfigPath("./config")
	viper.SetDefault("auto_download", true)
	viper.SetDefault("delete_url", true)
	viper.SetDefault("allowed_user", []int{})
	viper.SetDefault("allowed_chat", []int{})
	viper.SetDefault("yt-dlp_filter", "bv[height<=1080][vcodec^=avc1][filesize<500M]+ba[ext=m4a]/bv[height<=720][vcodec^=avc1][filesize<400M]+ba[ext=m4a]/bv[height<=480][vcodec^=avc1][filesize<300M]+ba[ext=m4a]/b[ext=mp4]")
	viper.SetDefault("duration", "600")
	viper.SetDefault("long_video_download", false)
	viper.SetDefault("live_filter", "!is_live & !was_live")
	viper.SetDefault("cache_max_age_days", 30)
	viper.SetDefault("cache_max_size_gb", 10)
	viper.SafeWriteConfig()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Println("Конфіг файл не знайдений")
		} else {
			log.Printf("Помилка з конфіг файлом: %v", err)
		}
	}
}

func Watch() {
	viper.OnConfigChange(func(e fsnotify.Event) {
		log.Printf("Конфігурація змінена: %s", e.Name)
		Reload()
	})
	viper.WatchConfig()
}

func ToggleBool(key string) {
	mu.Lock()
	defer mu.Unlock()

	newVal := !viper.GetBool(key)
	viper.Set(key, newVal)
	if err := viper.WriteConfig(); err != nil {
		viper.Set(key, !newVal)
		return
	}
	return
}
