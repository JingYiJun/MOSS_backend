package chat

import (
	"MOSS_backend/data"
	"MOSS_backend/models"
	"bytes"
	"context"
	"encoding/json"
	"github.com/chromedp/chromedp"
	"net/http"
	"net/http/httptest"
	"strings"
)

func GenerateImage(records []models.RecordModel) ([]byte, error) {
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		for i := range records {
			records[i].Request = strings.ReplaceAll(records[i].Request, "\n", "<br>")
			records[i].Response = strings.ReplaceAll(records[i].Response, "\n", "<br>")
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
