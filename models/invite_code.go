package models

type InviteCode struct {
	Code string `gorm:"primaryKey,size:32"`
}
