package sensitive

import (
	"MOSS_backend/config"
	"MOSS_backend/utils/sensitive/diting"
	"MOSS_backend/utils/sensitive/shumei"
)

func IsSensitive(content string) bool {
	if config.Config.SensitiveCheckPlatform == "ShuMei" {
		return shumei.IsSensitive(content)
	} else {
		return diting.IsSensitive(content)
	}
}
