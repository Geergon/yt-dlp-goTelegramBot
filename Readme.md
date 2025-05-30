# yt-dlp-goTelegramBot 🇺🇦

This is a small and simple bot for private chats, implemented using MTProto via [GoTGProto](https://github.com/celestix/gotgproto/tree/beta). This allows bypassing the BotAPI limitation on uploaded file sizes under 50 MB. The bot can download videos from YouTube, TikTok, and Instagram. The bot supports filtering videos by size, resolution, and other parameters, with flexible configuration via a `config.toml` file. It is designed for automated media downloading with customizable constraints.


## 💻 Installation and Usage with Docker Compose

### Prerequisites

-   Docker / Podman
-   Docker Compose / Podman Compose
-   Telegram Bot Token (obtain from [BotFather](https://t.me/BotFather))
-   Telegram API_HASH and API_ID (https://my.telegram.org/apps)

### Setup Steps

1. **Create a `compose.yaml` file (or download from the repository)**  in the project root with the following content:
    


```yaml
version: "3.8"

services:
  yt-dlp-gotelegrambot:
    image: geergon/yt-dlp-gotelegrambot:latest
    environment:
      APP_ID: ${APP_ID}
      API_HASH: ${API_HASH}
      BOT_TOKEN: ${BOT_TOKEN}
      CHAT_ID: ${CHAT_ID}
    volumes:
      - ./config:/config
      - ./db:/db
      - ./cookies:/cookies

```
You must have an .env file in the directory with compose.yaml
Environment Variables

    APP_ID: Telegram API ID (required).
    API_HASH: Telegram API hash (required).
    BOT_TOKEN: Telegram bot token from BotFather (required).
    CHAT_ID: The chat ID where the bot will send media (required).

Volumes

    ./config:/config: Directory for the config.yaml file and other configuration data.
    ./db:/db: Directory for storing bot session data (e.g., Telegram session files).
    ./cookies:/cookies: Directory for storing cookies files (e.g., cookiesYT.txt, cookiesTT.txt, cookiesINSTA.txt) used by yt-dlp for authentication.

If necessary, you can drop cookies into the cookies directory. The files should be named: 
**cookiesYT.txt** for YouTube, 
**cookiesTT.txt** for TikTok, 
**cookiesINSTA.txt** for Instagram.

2. **Run the bot using Docker/Podman Compose**:

```bash
    docker-compose up -d
```
```bash
    podman-compose up -d
```
3. **Usage**:
   
    -  Send a URL Paste a YouTube, TikTok, or Instagram URL. If auto_download is enabled, the bot will automatically download and send the media.
    -  /download: Manually trigger a download for the provided URL.
    -  /fragment <URL> <start-end>: Extract a video fragment (e.g., /fragment https://www.youtube.com/watch?v=XYZ 05:00-07:00).
    -  /audio: Download and send the audio version of the provided URL.
    -  /logs: Send bot logs in the chat
    -  /update: Update yt-dlp and gallery-dl version

### Stopping the Bot

To stop the bot:

```bash
docker-compose down

```

## ⚙ Configuration (`config.toml`)

The `config.toml` file contains settings for the bot's operation. Below is an explanation of each parameter:

-   **`allowed_chat`** (`list`): A list of Telegram chat IDs permitted to interact with the bot. If empty (`[]`), the bot is only available for the chat specified in CHAT_ID.
    
    -   Example: `allowed_chat = [123456789, -987654321]`
-   **`allowed_user`** (`list`): A list of Telegram user IDs permitted to interact with the bot. If empty (`[]`), no user can interact with the bot (This only applies to private chats with the bot, in allowed chats all users in chat can use the bot).
    
    -   Example: `allowed_user = [123456789]`
-   **`auto_download`** (`boolean`): Enables or disables automatic downloading of videos upon receiving a URL. If `true`, the bot downloads media automatically; if `false`, a command "/download url" is required to initiate downloading.
    
    -   Example: `auto_download = true`
-   **`delete_url`** (`boolean`): If `true`, the bot deletes the message containing the URL after processing. If `false`, the message remains in the chat.
    
    -   Example: `delete_url = false`
-   **`yt-dlp_filter`** (`string`): A filter for `yt-dlp` that specifies criteria for selecting videos (e.g., file size, resolution, format). In the example below, the bot prioritizes MP4 videos under 500 MB with M4A audio, then 720p videos under 400 MB, and finally 480p videos under 300 MB.
    
    -   Example: `yt-dlp_filter = 'bv[filesize<500M][ext=mp4]+ba[ext=m4a]/bv[height=720][filesize<400M][ext=mp4]+ba[ext=m4a]/bv[height=480][filesize<300M][ext=mp4]+ba[ext=m4a]'`
-   **`duration`** (`string`): If `long_video_download = false`, then this parameter sets the maximum length of the video that will be downloaded, by default 600 seconds (10 minutes).
    
    -   Example: `duration = '600'` (10 minutes)
-   **`long_video_download`** (`boolean`): If `true`, the bot allows downloading videos longer than the specified `duration`. If `false`, such videos are ignored.
    
    -   Example: `long_video_download = false`
 
### Example `config.toml`

```toml
allowed_chat = []
allowed_user = []
auto_download = true
delete_url = false
yt-dlp_filter = 'bv[filesize<500M][ext=mp4]+ba[ext=m4a]/bv[height=720][filesize<400M][ext=mp4]+ba[ext=m4a]/bv[height=480][filesize<300M][ext=mp4]+ba[ext=m4a]'
duration = '600'
long_video_download = false

```

## 📝 Additional Notes
  Parameters such as `auto_download`, `delete_url` and `long_video_download` can be changed directly in Telegram by typing the command "/settings". You can also change the configuration in the config.toml file without having to restart the bot.
