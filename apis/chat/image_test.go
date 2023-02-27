package chat

import (
	"MOSS_backend/models"
	"os"
	"testing"
)

func TestGenerateImage(t *testing.T) {
	buf, err := GenerateImage([]models.RecordModel{
		{"111", "```func\nfunc\nfunc\n```"},
		{"111", "```func\nfunc\nfunc\n```"},
	})
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile("screenshots/fullscreenshot.png", buf, 0644)
	if err != nil {
		t.Fatal(err)
	}
}
