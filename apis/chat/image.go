package chat

import (
	"MOSS_backend/data"
	"MOSS_backend/models"
	"context"
	"github.com/chromedp/chromedp"
	"net/http"
	"net/http/httptest"
	"strings"
	"text/template"
)

var imageTemplate, _ = template.New("image").Funcs(map[string]any{"replace": ContentProcess}).Parse(string(data.ImageTemplate))

func GenerateImage(records []models.RecordModel) ([]byte, error) {
	// disable javascript in headless chrome
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("blink-settings", "scriptEnabled=false"),
	)
	ctx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	ctx, cancel = chromedp.NewContext(context.Background())
	defer cancel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_ = imageTemplate.Execute(w, struct {
			Records []models.RecordModel
		}{
			Records: records,
		})
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

func ContentProcess(content string) string {
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
