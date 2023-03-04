package chat

import (
	"MOSS_backend/config"
	"MOSS_backend/data"
	"MOSS_backend/models"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/chromedp/chromedp"
	"net/http"
	"net/http/httptest"
	"strings"
)

func GenerateImage(records []models.RecordModel) ([]byte, error) {
	if !config.Config.OpenScreenshot {
		return nil, errors.New("截图功能暂缓开放")
	}
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		for i := range records {
			records[i].Request = contentProcess(records[i].Request)
			records[i].Response = contentProcess(records[i].Response)
		}
		recordsData, _ := json.Marshal(records)
		_, _ = w.Write(bytes.Replace(
			data.ImageTemplate,
			[]byte(`[{"request": "你好", "response": "你好，很高兴认识你，我是moss，一个聊天助手，巴拉巴拉巴拉巴拉，"}]`),
			recordsData,
			1,
		))
	}))
	defer server.Close()

	var buf []byte
	err := chromedp.Run(ctx,
		chromedp.EmulateViewport(800, 200),
		chromedp.Navigate(server.URL),
		chromedp.FullScreenshot(&buf, 100),
	)
	return buf, err
}

func contentProcess(content string) string {
	recordLines := strings.Split(content, "\n")
	var builder strings.Builder
	for i, recordLine := range recordLines {
		prefixSpaceCount := 0
		for _, character := range recordLine {
			if character == ' ' {
				prefixSpaceCount++
			} else {
				break
			}
		}

		if prefixSpaceCount == 0 {
			builder.WriteString(recordLine)
		} else {
			builder.WriteString(strings.Replace(recordLine, " ", "&nbsp;", prefixSpaceCount))
		}
		if i != len(recordLines)-1 {
			builder.WriteString("<br>")
		}
	}
	return builder.String()
}
