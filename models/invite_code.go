package models

type InviteCode struct {
	ID          int    `gorm:"primaryKey"`
	Code        string `gorm:"unique,size:32"`
	IsSend      bool
	IsActivated bool
}
