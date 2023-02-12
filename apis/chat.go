package apis

import (
	. "MOSS_backend/models"
	. "MOSS_backend/utils"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// ListChats
// @Summary list user's chats
// @Tags chat
// @Router /chats [get]
// @Success 200 {array} models.Chat
func ListChats(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return err
	}

	var chats = Chats{}
	err = DB.Order("updated_at desc").Find(&chats, "user_id = ?", userID).Error
	if err != nil {
		return err
	}

	return c.JSON(chats)
}

// AddChat
// @Summary add a chat
// @Tags chat
// @Router /chats [post]
// @Success 201 {object} models.Chat
func AddChat(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return err
	}

	chat := Chat{UserID: userID}
	err = DB.Create(&chat).Error
	if err != nil {
		return err
	}

	return c.Status(201).JSON(chat)
}

// ModifyChat
// @Summary modify a chat
// @Tags chat
// @Router /chats/{chat_id} [put]
// @Param chat_id path int true "chat id"
// @Param json body ChatModifyModel true "json"
// @Success 200 {object} models.Chat
func ModifyChat(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return err
	}

	chatID, err := c.ParamsInt("id")
	if err != nil {
		return err
	}

	var body ChatModifyModel
	err = ValidateBody(c, &body)
	if err != nil {
		return err
	}

	var chat Chat
	err = DB.Transaction(func(tx *gorm.DB) error {
		err = tx.Clauses(LockingClause).Take(&chat, chatID).Error
		if err != nil {
			return err
		}

		if chat.UserID != userID {
			return Forbidden()
		}

		if body.Name != nil {
			chat.Name = *body.Name
		}

		return tx.Save(&chat).Error
	})
	if err != nil {
		return err
	}

	return c.JSON(chat)
}

// DeleteChat
// @Summary delete a chat
// @Tags chat
// @Router /chats/{chat_id} [delete]
// @Param chat_id path int true "chat id"
// @Success 204
func DeleteChat(c *fiber.Ctx) error {
	userID, err := GetUserID(c)
	if err != nil {
		return err
	}

	chatID, err := c.ParamsInt("id")
	if err != nil {
		return err
	}

	var chat Chat
	err = DB.Transaction(func(tx *gorm.DB) error {
		err = tx.Clauses(LockingClause).Take(&chat, chatID).Error
		if err != nil {
			return err
		}

		if chat.UserID != userID {
			return Forbidden()
		}

		return tx.Delete(&chat).Error
	})
	if err != nil {
		return err
	}

	return c.SendStatus(204)
}

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
// @Param json body RecordCreateModel true "json"
// @Success 201 {object} models.Record
func AddRecord(c *fiber.Ctx) error {
	chatID, err := c.ParamsInt("id")
	if err != nil {
		return err
	}

	// validate body
	var body RecordCreateModel
	err = ValidateBody(c, &body)
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
	} // not exists

	// permission
	if chat.UserID != userID {
		return Forbidden()
	}

	record := Record{
		ChatID:  chatID,
		Request: body.Request,
	}

	// sensitive request check
	if IsSensitive(record.Request) {
		record.RequestSensitive = true
		record.Response = DefaultResponse
	} else {
		/* infer */
		// get all params to infer server
		var params Params
		err = DB.Find(&params).Error
		if err != nil {
			return err
		}

		// find all records to make dialogs, without sensitive content
		var records Records
		err = DB.Find(&records, "chat_id = ? and request_sensitive <> true and response_sensitive <> true", chatID).Error
		if err != nil {
			return err
		}

		// infer request
		record.Response, record.Duration, err = Infer(InferRequest{
			Records: records.ToRecordModel(),
			Message: record.Request,
			Params:  params,
		})
		if err != nil {
			return err
		}

		if IsSensitive(record.Response) {
			record.ResponseSensitive = true
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
			chat.Name = record.Request
		}
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

	userID, err := GetUserID(c)
	if err != nil {
		return err
	}

	var chat Chat
	err = DB.Take(&chat, chatID).Error
	if err != nil {
		return err
	}

	// permission
	if chat.UserID != userID {
		return Forbidden()
	}

	// get the latest record
	var oldRecord Record
	err = DB.Last(&oldRecord, "chat_id = ?", chat.ID).Error
	if err != nil {
		return err
	}

	if oldRecord.RequestSensitive {
		// old record request is sensitive
		return Serialize(c, &oldRecord)
	}

	record := Record{
		ChatID:  chatID,
		Request: oldRecord.Request,
	}

	/* infer */
	// get all params to infer server
	var params Params
	err = DB.Find(&params).Error
	if err != nil {
		return err
	}

	// find all records to make dialogs, without sensitive content
	var records Records
	err = DB.Find(&records, "chat_id = ? and request_sensitive <> true and response_sensitive <> true", chatID).Error
	if err != nil {
		return err
	}

	// remove the latest record
	if len(records) > 0 {
		records = records[0 : len(records)-1]
	}

	// infer request
	record.Response, record.Duration, err = Infer(InferRequest{
		Records: records.ToRecordModel(),
		Message: record.Request,
		Params:  params,
	})
	if err != nil {
		return err
	}

	if IsSensitive(record.Response) {
		record.ResponseSensitive = true
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
// @Param json body RecordModifyModel true "json"
// @Success 201 {object} models.Record
func ModifyRecord(c *fiber.Ctx) error {
	recordID, err := c.ParamsInt("id")
	if err != nil {
		return err
	}

	var body RecordModifyModel
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

		return tx.Save(&record).Error
	})

	if err != nil {
		return err
	}

	return c.JSON(&record)
}
