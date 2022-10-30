package bot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var recognizeAPI = "https://music-recognize.vercel.app/api/recognize"

func recognizeMusic(message tgbotapi.Message, bot *tgbotapi.BotAPI) (err error) {
	if message.ReplyToMessage == nil {
		msg := tgbotapi.NewMessage(message.Chat.ID, "请回复一条语音留言")
		msg.ReplyToMessageID = message.MessageID
		_, err = bot.Send(msg)
		return err
	}
	if message.ReplyToMessage.Voice == nil {
		msg := tgbotapi.NewMessage(message.Chat.ID, "请回复一条语音留言")
		msg.ReplyToMessageID = message.ReplyToMessage.MessageID
		_, err = bot.Send(msg)
		return err
	}

	tempBot := tgbotapi.BotAPI{
		Token:  bot.Token,
		Client: &http.Client{},
	}
	tempBot.SetAPIEndpoint(tgbotapi.APIEndpoint)
	url, err := tempBot.GetFileDirectURL(message.ReplyToMessage.Voice.FileID)
	if err != nil {
		return err
	}

	buf, err := http.Get(url)
	if err != nil {
		return err
	}

	fileName := fmt.Sprintf("%d-%d-%d.ogg", message.ReplyToMessage.Chat.ID, message.ReplyToMessage.MessageID, time.Now().Unix())
	file, err := os.OpenFile(fmt.Sprintf("%s/%s", cacheDir, fileName), os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return err
	}

	_, err = io.Copy(file, buf.Body)
	if err != nil {
		return err
	}
	file.Close()

	// convert ogg to mp3
	cmd := exec.Command("ffmpeg", "-i", fmt.Sprintf("%s/%s", cacheDir, fileName), fmt.Sprintf("%s/%s.mp3", cacheDir, fileName))
	err = cmd.Run()
	if err != nil {
		//return err
	}
	_, err = os.Stat(fmt.Sprintf("%s/%s.mp3", cacheDir, fileName))
	if err != nil {
		return err
	}

	newFile, err := os.Open(fmt.Sprintf("%s/%s.mp3", cacheDir, fileName))
	newBuf, _ := io.ReadAll(newFile)

	resp, err := uploadFile(recognizeAPI, newBuf)
	if err != nil {
		return err
	}

	var result RecognizeResultData
	err = json.Unmarshal(resp, &result)

	if err != nil && len(result.Data.Result) == 0 {
		msg := tgbotapi.NewMessage(message.Chat.ID, "识别失败，可能是录音时间太短")
		msg.ReplyToMessageID = message.ReplyToMessage.MessageID
		_, _ = bot.Send(msg)
		return err
	}

	musicID := result.Data.Result[0].Song.Id
	if musicID == 0 {
		msg := tgbotapi.NewMessage(message.Chat.ID, "识别失败，未知错误")
		msg.ReplyToMessageID = message.ReplyToMessage.MessageID
		_, err = bot.Send(msg)
		return err
	}

	msg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("https://music.163.com/song/%d", musicID))
	msg.ReplyToMessageID = message.ReplyToMessage.MessageID
	_, _ = bot.Send(msg)
	return processMusic(musicID, *message.ReplyToMessage, bot)
}

func uploadFile(url string, file []byte) ([]byte, error) {
	bodyBuf := &bytes.Buffer{}
	bodyWriter := multipart.NewWriter(bodyBuf)
	fileWriter, err := bodyWriter.CreateFormFile("file", "")
	if err != nil {
		return nil, err
	}
	_, err = fileWriter.Write(file)
	if err != nil {
		return nil, err
	}
	contentType := bodyWriter.FormDataContentType()
	bodyWriter.Close()
	resp, err := http.Post(url, contentType, bodyBuf)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	return respBody, nil
}

type RecognizeResultData struct {
	Data struct {
		Result []struct {
			Song struct {
				Name string `json:"name"`
				Id   int    `json:"id"`
			} `json:"song"`
		} `json:"result"`
	} `json:"data"`
	Code    int    `json:"code"`
	Message string `json:"message"`
}
