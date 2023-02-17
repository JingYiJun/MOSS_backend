package models

import (
	"MOSS_backend/config"
	"MOSS_backend/utils"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
	"golang.org/x/exp/slices"
	"gorm.io/gorm"
	"strconv"
	"strings"
	"time"
)

type User struct {
	ID           int            `json:"id" gorm:"primaryKey"`
	JoinedTime   time.Time      `json:"joined_time" gorm:"autoCreateTime"`
	LastLogin    time.Time      `json:"last_login" gorm:"autoUpdateTime"`
	DeletedAt    gorm.DeletedAt `json:"-" gorm:"index"`
	Nickname     string         `json:"nickname" gorm:"size:128;default:'user'"`
	Email        string         `json:"email" gorm:"size:128;index:,length:5"`
	Phone        string         `json:"phone" gorm:"size:128;index:,length:5"`
	Password     string         `json:"-" gorm:"size:128"`
	RegisterIP   string         `json:"-" gorm:"size:32"`
	LastLoginIP  string         `json:"-" gorm:"size:32"`
	LoginIP      []string       `json:"-" gorm:"serializer:json"`
	Chats        Chats          `json:"chats,omitempty"`
	ShareConsent bool           `json:"share_consent" gorm:"default:true"`
	InviteCode   string         `json:"-" gorm:"size:32"`
}

func GetUserID(c *fiber.Ctx) (int, error) {
	if config.Config.Mode == "dev" || config.Config.Mode == "test" {
		return 1, nil
	}

	id, err := strconv.Atoi(c.Get("X-Consumer-Username"))
	if err != nil {
		return 0, utils.Unauthorized("Unauthorized")
	}

	return id, nil
}

func GetUserIDFromWs(c *websocket.Conn) (int, error) {
	// get cookie named access or query jwt
	token := c.Query("jwt")
	if token == "" {
		token = c.Cookies("access")
		if token == "" {
			return 0, utils.Unauthorized()
		}
	}
	// get data
	data, err := parseJWT(token, false)
	if err != nil {
		return 0, err
	}
	id, ok := data["id"] // get id
	if !ok {
		return 0, utils.Unauthorized()
	}
	return int(id.(float64)), nil
}

// parseJWT extracts and parse token
func parseJWT(token string, bearer bool) (Map, error) {
	if len(token) < 7 {
		return nil, errors.New("bearer token required")
	}

	if bearer {
		token = token[7:]
	}

	payloads := strings.SplitN(token[7:], ".", 3) // extract "Bearer "
	if len(payloads) < 3 {
		return nil, errors.New("jwt token required")
	}

	// jwt encoding ignores padding, so RawStdEncoding should be used instead of StdEncoding
	payloadBytes, err := base64.RawStdEncoding.DecodeString(payloads[1]) // the middle one is payload
	if err != nil {
		return nil, err
	}

	var value Map
	err = json.Unmarshal(payloadBytes, &value)
	return value, err
}

func GetUserByRefreshToken(c *fiber.Ctx) (*User, error) {
	// get id
	userID, err := GetUserID(c)
	if err != nil {
		return nil, err
	}

	tokenString := c.Get("Authorization")
	if tokenString == "" { // token can be in either header or cookie
		tokenString = c.Cookies("refresh")
	}

	payload, err := parseJWT(tokenString, true)
	if err != nil {
		return nil, err
	}

	if tokenType, ok := payload["type"]; !ok || tokenType != "refresh" {
		return nil, utils.Unauthorized("refresh token invalid")
	}

	var user User
	err = DB.Take(&user, userID).Error
	return &user, err
}

func (user *User) UpdateIP(ip string) {
	user.LastLoginIP = ip
	if !slices.Contains(user.LoginIP, ip) {
		user.LoginIP = append(user.LoginIP, ip)
	}
}
