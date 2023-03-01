package sensitive

import (
	"MOSS_backend/config"
	"MOSS_backend/models"
	"MOSS_backend/utils/sensitive/diting"
	"MOSS_backend/utils/sensitive/shumei"
)

func IsSensitive(content string, user *models.User) bool {
	if user.IsAdmin {
		if user.DisableSensitiveCheck {
			return false
		}
	}
	if config.Config.SensitiveCheckPlatform == "ShuMei" {
		return shumei.IsSensitive(content)
	} else {
		return diting.IsSensitive(content)
	}
}
