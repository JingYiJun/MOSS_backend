package record

import (
	"MOSS_backend/config"
	. "MOSS_backend/models"
	. "MOSS_backend/utils"
	"MOSS_backend/utils/sensitive"
	"errors"
	"github.com/gofiber/fiber/v2"
	"golang.org/x/exp/slices"
	"gorm.io/gorm"
	"time"
)

// ListRecords
// @Summary list records of a chat
// @Tags record
// @Router /chats/{chat_id}/records [get]
// @Param chat_id path int true "chat id"
// @Success 200 {array} models.Record
func ListRecords(c *fiber.Ctx) error {
	chatID, err := c.ParamsInt("id")
	if err != nil {
		return err
	}

	userID, err := GetUserID(c)
	if err != nil {
		return err
	}

	var chat Chat
	err = DB.Take(&chat, chatID).Error
	if err != nil {
		return err
	}

	if userID != chat.UserID {
		return Forbidden()
	}

	var records = Records{}
	err = DB.Find(&records, "chat_id = ?", chatID).Error
	if err != nil {
		return err
	}

	return Serialize(c, records)
}

// AddRecord
// @Summary add a record
// @Tags record
// @Router /chats/{chat_id}/records [post]
// @Param chat_id path int true "chat id"
// @Param json body CreateModel true "json"
// @Success 201 {object} models.Record
func AddRecord(c *fiber.Ctx) error {
	chatID, err := c.ParamsInt("id")
	if err != nil {
		return err
	}

	// validate body
	var body CreateModel
	err = ValidateBody(c, &body)
	if err != nil {
		return err
	}

	if body.Request == "" {
		return BadRequest("request is empty")
	} else if len([]rune(body.Request)) > 1000 {
		return maxInputExceededError
	}

	user, err := LoadUser(c)
	if err != nil {
		return err
	}

	// check user lock
	if _, ok := userLockMap.LoadOrStore(user.ID, UserLockValue{LockTime: time.Now()}); ok {
		return userRequestingError
	}
	defer userLockMap.Delete(user.ID)

	// infer limiter
	if !inferLimiter.Allow() {
		return unknownError
	}

	banned, err := user.CheckUserOffense()
	if err != nil {
		return err
	}
	if banned {
		return Forbidden(OffenseMessage)
	}

	var chat Chat
	err = DB.Take(&chat, chatID).Error
	if err != nil {
		return err
	} // not exists

	// permission
	if chat.UserID != user.ID {
		return Forbidden()
	}

	record := Record{
		ChatID:  chatID,
		Request: body.Request,
	}

	// sensitive request check
	if sensitive.IsSensitive(record.Request, user) {
		record.RequestSensitive = true
		record.Response = DefaultResponse

		banned, err = user.AddUserOffense(UserOffensePrompt)
		if err != nil {
			return err
		}
		if banned {
			return Forbidden(OffenseMessage)
		}
	} else {
		/* infer */

		// find last record prefix to make dialogs, without sensitive content
		var oldRecord Record
		err = DB.Last(&oldRecord, "chat_id = ? AND request_sensitive = ? AND response_sensitive = ?", chatID, false, false).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		// infer request
		err = Infer(&record, oldRecord.Prefix, user, body.Param)
		if err != nil {
			//if errors.Is(err, maxLengthExceededError) {
			//	chat.MaxLengthExceeded = true
			//	DB.Save(&chat)
			//}
			return err
		}

		if sensitive.IsSensitive(record.Response, user) {
			record.ResponseSensitive = true

			banned, err = user.AddUserOffense(UserOffenseMoss)
			if err != nil {
				return err
			}
			if banned {
				return Forbidden(OffenseMessage)
			}
		}
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err = tx.Clauses(LockingClause).Take(&chat, chatID).Error
		if err != nil {
			return err
		}

		err = tx.Create(&record).Error
		if err != nil {
			return err
		}

		if chat.Count == 0 {
			chat.Name = StripContent(record.Request, config.Config.ChatNameLength)
		}
		chat.Count += 1
		return tx.Save(&chat).Error
	})
	if err != nil {
		return err
	}

	return Serialize(c.Status(201), &record)
}

// RetryRecord
// @Summary regenerate the last record of a chat
// @Tags record
// @Router /chats/{chat_id}/regenerate [put]
// @Param chat_id path int true "chat id"
// @Success 200 {object} models.Record
func RetryRecord(c *fiber.Ctx) error {
	chatID, err := c.ParamsInt("id")
	if err != nil {
		return err
	}

	user, err := LoadUser(c)
	if err != nil {
		return err
	}

	var chat Chat
	err = DB.Take(&chat, chatID).Error
	if err != nil {
		return err
	}

	// check user lock
	if _, ok := userLockMap.LoadOrStore(user.ID, UserLockValue{LockTime: time.Now()}); ok {
		return userRequestingError
	}
	defer userLockMap.Delete(user.ID)

	// infer limiter
	if !inferLimiter.Allow() {
		return unknownError
	}

	banned, err := user.CheckUserOffense()
	if err != nil {
		return err
	}
	if banned {
		return Forbidden(OffenseMessage)
	}

	// permission
	if chat.UserID != user.ID {
		return Forbidden()
	}

	// get the latest record
	var oldRecord Record
	err = DB.Last(&oldRecord, "chat_id = ?", chat.ID).Error
	if err != nil {
		return err
	}

	if !user.IsAdmin || !user.DisableSensitiveCheck {
		if oldRecord.RequestSensitive {
			banned, err = user.AddUserOffense(UserOffensePrompt)
			if err != nil {
				return err
			}
			if banned {
				return Forbidden(OffenseMessage)
			}

			// old record request is sensitive
			return Serialize(c, &oldRecord)
		}
	}

	record := Record{
		ChatID:  chatID,
		Request: oldRecord.Request,
	}

	/* infer */

	// find last record prefix to make dialogs, without sensitive content
	var prefixRecord Record
	err = DB.Last(&prefixRecord, "chat_id = ? AND request_sensitive = false AND response_sensitive = false AND id < ?", chatID, oldRecord.ID).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	// infer request
	err = Infer(&record, prefixRecord.Prefix, user, nil)
	if err != nil {
		//if errors.Is(err, maxLengthExceededError) {
		//	chat.MaxLengthExceeded = true
		//	DB.Save(&chat)
		//}
		return err
	}

	if sensitive.IsSensitive(record.Response, user) {
		record.ResponseSensitive = true

		banned, err = user.AddUserOffense(UserOffenseMoss)
		if err != nil {
			return err
		}
		if banned {
			return Forbidden(OffenseMessage)
		}
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err = tx.Clauses(LockingClause).Take(&chat, chatID).Error
		if err != nil {
			return err
		}

		err = tx.Delete(&oldRecord).Error
		if err != nil {
			return err
		}

		err = tx.Create(&record).Error
		if err != nil {
			return err
		}

		return tx.Save(&chat).Error
	})
	if err != nil {
		return err
	}

	return Serialize(c, &record)
}

// ModifyRecord
// @Summary modify a record
// @Tags record
// @Router /records/{record_id} [put]
// @Param record_id path int true "record id"
// @Param json body ModifyModel true "json"
// @Success 201 {object} models.Record
func ModifyRecord(c *fiber.Ctx) error {
	recordID, err := c.ParamsInt("id")
	if err != nil {
		return err
	}

	var body ModifyModel
	err = ValidateBody(c, &body)
	if err != nil {
		return err
	}

	userID, err := GetUserID(c)
	if err != nil {
		return err
	}

	if body.Feedback == nil && body.Like == nil {
		return BadRequest()
	}

	var record Record
	err = DB.Transaction(func(tx *gorm.DB) error {
		var chat Chat
		err = tx.Clauses(LockingClause).Take(&record, recordID).Error
		if err != nil {
			return err
		}

		err = tx.Take(&chat, record.ChatID).Error
		if err != nil {
			return err
		}

		if chat.UserID != userID {
			return Forbidden()
		}

		if body.Feedback != nil {
			record.Feedback = *body.Feedback
		}

		if body.Like != nil {
			record.LikeData = *body.Like
		}

		return tx.Model(&record).Select("Feedback", "LikeData").Updates(&record).Error
	})

	if err != nil {
		return err
	}

	return Serialize(c, &record)
}

// InferWithoutLogin
// @Summary infer without login
// @Tags Inference
// @Router /inference [post]
// @Param json body InferenceRequest true "json"
// @Success 200 {object} InferenceResponse
func InferWithoutLogin(c *fiber.Ctx) error {
	var body InferenceRequest
	err := ValidateBody(c, &body)
	if err != nil {
		return err
	}

	if body.Request == "" {
		return BadRequest("request is empty")
	} else if len([]rune(body.Request)) > 1000 {
		return maxInputExceededError
	}

	// infer limiter
	if !inferLimiter.Allow() {
		return unknownError
	}

	consumerUsername := c.Get("X-Consumer-Username")
	passSensitiveCheck := slices.Contains(config.Config.PassSensitiveCheckUsername, consumerUsername)

	if !passSensitiveCheck && sensitive.IsSensitive(body.Context, &User{}) {
		return BadRequest(DefaultResponse).WithMessageType(Sensitive)
	}

	record := Record{Request: body.Request}

	err = Infer(&record, body.Context, &User{PluginConfig: body.PluginConfig}, body.Param)
	if err != nil {
		return err
	}

	if !passSensitiveCheck && sensitive.IsSensitive(record.Response, &User{}) {
		return BadRequest(DefaultResponse).WithMessageType(Sensitive)
	}

	directRecord := DirectRecord{
		Duration:         record.Duration,
		ConsumerUsername: consumerUsername,
		Context:          record.Prefix,
		Request:          record.Request,
		Response:         record.Response,
		ExtraData:        record.ExtraData,
	}

	_ = DB.Create(&directRecord).Error

	return c.JSON(InferenceResponse{
		Response:  record.Response,
		Context:   record.Prefix,
		ExtraData: record.ExtraData,
	})
}
