package models

type InviteCode struct {
	ID          string `gorm:"primaryKey"`
	Code        string `gorm:"unique,size:32"`
	IsSend      bool
	IsActivated bool
}
