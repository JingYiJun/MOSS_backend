package models

import (
	"MOSS_backend/config"
	"MOSS_backend/utils"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"
	"gorm.io/gorm"
)

type User struct {
	ID                    int             `json:"id" gorm:"primaryKey"`
	JoinedTime            time.Time       `json:"joined_time" gorm:"autoCreateTime"`
	LastLogin             time.Time       `json:"last_login" gorm:"autoUpdateTime"`
	DeletedAt             gorm.DeletedAt  `json:"-" gorm:"index"`
	Nickname              string          `json:"nickname" gorm:"size:128;default:'user'"`
	Email                 string          `json:"email" gorm:"size:128;index:,length:5"`
	Phone                 string          `json:"phone" gorm:"size:128;index:,length:5"`
	Password              string          `json:"-" gorm:"size:128"`
	RegisterIP            string          `json:"-" gorm:"size:32"`
	LastLoginIP           string          `json:"-" gorm:"size:32"`
	LoginIP               []string        `json:"-" gorm:"serializer:json"`
	Chats                 Chats           `json:"chats,omitempty"`
	ShareConsent          bool            `json:"share_consent" gorm:"default:true"`
	InviteCode            string          `json:"-" gorm:"size:32"`
	IsAdmin               bool            `json:"is_admin"`
	DisableSensitiveCheck bool            `json:"disable_sensitive_check"`
	Banned                bool            `json:"banned"`
	ModelID               int             `json:"model_id" default:"1" gorm:"default:1"`
	PluginConfig          map[string]bool `json:"plugin_config" gorm:"serializer:json"`
}

func GetUserCacheKey(userID int) string {
	return "moss_user:" + strconv.Itoa(userID)
}

const UserCacheExpire = 48 * time.Hour

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

// LoadUserByIDFromCache return value `err` is directly from DB.Take()
func LoadUserByIDFromCache(userID int, userPtr *User) error {
	cacheKey := GetUserCacheKey(userID)
	if config.GetCache(cacheKey, userPtr) != nil {
		err := DB.Take(userPtr, userID).Error
		if err != nil {
			return err
		}
		// err has been printed in SetCache
		_ = config.SetCache(cacheKey, *userPtr, UserCacheExpire)
	}
	return nil
}

func DeleteUserCacheByID(userID int) {
	cacheKey := GetUserCacheKey(userID)
	err := config.DeleteCache(cacheKey)
	if err != nil {
		utils.Logger.Error("err in DeleteUserCacheByID: ", zap.Error(err))
	}
}

func LoadUserByID(userID int) (*User, error) {
	var user User
	err := LoadUserByIDFromCache(userID, &user)
	if err != nil { // something wrong in DB.Take() in LoadUserByIDFromCache()
		DeleteUserCacheByID(userID)
		return nil, err
	}
	updated := false

	if user.ModelID == 0 {
		user.ModelID = 1
		updated = true
	}

	var defaultPluginConfig map[string]bool
	defaultPluginConfig, err = GetPluginConfig(user.ModelID)

	if user.PluginConfig == nil {
		user.PluginConfig = make(map[string]bool)
		for key := range defaultPluginConfig {
			user.PluginConfig[key] = false
		}
		updated = true
	} else { // add new key
		for key := range defaultPluginConfig {
			if _, ok := user.PluginConfig[key]; !ok {
				user.PluginConfig[key] = false
				updated = true
			}
		}

		// delete not used key
		for key := range user.PluginConfig {
			if _, ok := defaultPluginConfig[key]; !ok {
				delete(user.PluginConfig, key)
				updated = true
			}
		}
	}

	if updated {
		DB.Model(&user).Select("ModelID", "PluginConfig").Updates(&user)
		err = config.SetCache(GetUserCacheKey(userID), user, UserCacheExpire)
	}
	return &user, err
}

func LoadUser(c *fiber.Ctx) (*User, error) {
	userID, err := GetUserID(c)
	if err != nil {
		return nil, err
	}
	return LoadUserByID(userID)
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

func LoadUserFromWs(c *websocket.Conn) (*User, error) {
	userID, err := GetUserIDFromWs(c)
	if err != nil {
		return nil, err
	}
	return LoadUserByID(userID)
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
	payloadBytes, err := base64.RawURLEncoding.DecodeString(payloads[1]) // the middle one is payload
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
	err = LoadUserByIDFromCache(userID, &user)
	return &user, err
}

func (user *User) UpdateIP(ip string) {
	user.LastLoginIP = ip
	if !slices.Contains(user.LoginIP, ip) {
		user.LoginIP = append(user.LoginIP, ip)
	}
}
