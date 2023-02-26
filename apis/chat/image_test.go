package chat

import (
	"MOSS_backend/models"
	"os"
	"testing"
)

func TestGenerateImage(t *testing.T) {
	buf, err := GenerateImage([]models.RecordModel{
		{"111", "1333"},
		{"111", "1444"},
	})
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile("fullscreenshot.png", buf, 0644)
	if err != nil {
		t.Fatal(err)
	}
}
