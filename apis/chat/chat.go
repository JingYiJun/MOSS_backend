package chat

import (
	. "MOSS_backend/models"
	. "MOSS_backend/utils"
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"os"
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

	// delete empty chats
	err = DB.Where("user_id = ? and count = 0", userID).Delete(&Chat{}).Error
	if err != nil {
		return err
	}

	// get all chats
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
// @Param json body ModifyModel true "json"
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

	var body ModifyModel
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

// GenerateChatScreenshot
// @Summary screenshot of a chat
// @Tags record
// @Produce png
// @Router /chats/{chat_id}/screenshots [get]
// @Param chat_id path int true "chat id"
// @Success 200
func GenerateChatScreenshot(c *fiber.Ctx) error {
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

	var records Records
	err = DB.Find(&records, "chat_id = ? and request_sensitive <> true and response_sensitive <> true", chatID).Error
	if err != nil {
		return err
	}

	buf, err := GenerateImage(records.ToRecordModel())
	if err != nil {
		return err
	}

	filename := uuid.NewString() + ".png"
	err = os.WriteFile(fmt.Sprintf("./screenshots/%s", filename), buf, 0644)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://%s/api/screenshots/%s", c.Get("Host"), filename)
	return c.JSON(Map{"url": url})
}
