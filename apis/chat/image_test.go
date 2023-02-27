package chat

import (
	"MOSS_backend/models"
	"os"
	"testing"
)

func TestGenerateImage(t *testing.T) {
	buf, err := GenerateImage([]models.RecordModel{
		{"111", "```func\n    func\n    func\n```"},
		{"111", "```func\nfunc\nfunc\n```"},
		{"hi", "My name is MOSS, created by the FudanNLP Lab in the School of Computer Science at Fudan University."},
	})
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile("screenshots/fullscreenshot.png", buf, 0644)
	if err != nil {
		t.Fatal(err)
	}
}
