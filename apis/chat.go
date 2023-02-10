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
	err = DB.Find(&chats, "user_id = ?", userID).Error
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

	return c.JSON(records)
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

	var body RecordCreateModel
	err = ValidateBody(c, &body)
	if err != nil {
		return err
	}

	userID, err := GetUserID(c)
	if err != nil {
		return err
	}

	var record Record
	err = DB.Transaction(func(tx *gorm.DB) error {
		var chat Chat
		err = tx.Clauses(LockingClause).Take(&chat, chatID).Error
		if err != nil {
			return err
		}

		if chat.UserID != userID {
			return Forbidden()
		}

		// get all params to infer server
		var params Params
		err = tx.Find(&params).Error
		if err != nil {
			return err
		}

		record.ChatID = chat.ID
		record.Request = body.Request

		// find all records to make dialogs
		var records Records
		err = tx.Find(&records, "chat_id = ?", chatID).Error
		if err != nil {
			return err
		}

		record.Response, record.Duration, err = Infer(InferRequest{
			Records: records.ToRecordModel(),
			Message: record.Request,
			Params:  params,
		})
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
		chat.Count += 1
		return tx.Save(&chat).Error
	})
	if err != nil {
		return err
	}

	return c.Status(201).JSON(record)
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

	var record Record
	err = DB.Transaction(func(tx *gorm.DB) error {
		var chat Chat
		err = tx.Clauses(LockingClause).Take(&chat, chatID).Error
		if err != nil {
			return err
		}

		if chat.UserID != userID {
			return Forbidden()
		}

		var oldRecord Record
		err = tx.Last(&oldRecord, "chat_id = ?", chat.ID).Error
		if err != nil {
			return err
		}

		// get all params to infer server
		var params Params
		err = tx.Find(&params).Error
		if err != nil {
			return err
		}

		record.ChatID = chatID
		record.Request = oldRecord.Request

		// find all records to make dialogs
		var records Records
		err = tx.Find(&records, "chat_id = ?", chatID).Error
		if err != nil {
			return err
		}

		record.Response, record.Duration, err = Infer(InferRequest{
			Records: records.ToRecordModel(),
			Message: record.Request,
			Params:  params,
		})
		if err != nil {
			return err
		}

		err = tx.Delete(&oldRecord).Error
		if err != nil {
			return err
		}

		return tx.Create(&record).Error
	})
	if err != nil {
		return err
	}

	return c.JSON(record)
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
		err = tx.Clauses(LockingClause).Take(&record, recordID).Error
		if err != nil {
			return err
		}

		var chat Chat
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
