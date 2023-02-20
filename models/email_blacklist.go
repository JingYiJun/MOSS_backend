package models

import (
	"golang.org/x/exp/slices"
	"strings"
)

type EmailBlacklist struct {
	ID          int
	EmailDomain string
}

func IsEmailInBlacklist(email string) bool {
	var blacklist []string
	DB.Model(&EmailBlacklist{}).Select("email_domain").Scan(&blacklist)
	parts := strings.Split(email, "@")
	return slices.Contains(blacklist, parts[1])
}
