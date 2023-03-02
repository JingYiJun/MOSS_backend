package models

type Config struct {
	ID             int
	InviteRequired bool
	OffenseCheck   bool
	Notice         string
}
