package models

import "time"

type Chat struct {
	ID        int       `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	UserID    int       `json:"user_id"`
	Count     int       `json:"count"` // Record 条数
	Records   Records   `json:"records,omitempty"`
}

type Chats []Chat

type Record struct {
	ID        int       `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	ChatID    int       `json:"chat_id"`
	Request   string    `json:"request"`
	Response  string    `json:"response"`
	LikeData  int       `json:"like_data"` // 1 like, -1 dislike
	Feedback  string    `json:"feedback"`
}

type Records []Record
