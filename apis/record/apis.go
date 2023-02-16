package record

import (
	"MOSS_backend/config"
	. "MOSS_backend/models"
	. "MOSS_backend/utils"
	"context"
	"encoding/json"
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
	"gorm.io/gorm"
	"log"
	"strconv"
	"strings"
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

		// find all records to make dialogs, without sensitive content
		var records Records
		err = DB.Find(&records, "chat_id = ? and request_sensitive <> true and response_sensitive <> true", chatID).Error
		if err != nil {
			return err
		}

		// infer request
		record.Response, record.Duration, err = Infer(record.Request, records)
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

// AddRecordAsync
// @Summary add a record
// @Tags Websocket
// @Router /ws/chats/{chat_id}/records [get]
// @Param chat_id path int true "chat id"
// @Param json body CreateModel true "json"
// @Success 201 {object} models.Record
func AddRecordAsync(c *websocket.Conn) {
	var (
		chatID  int
		userID  int
		message []byte
		err     error
	)

	defer func() {
		if err != nil {
			log.Println(err)
			response := InferResponseModel{Status: -1, Output: err.Error()}
			if httpError, ok := err.(*HttpError); ok {
				response.StatusCode = httpError.Code
			}
			_ = c.WriteJSON(response)
		}
	}()

	procedure := func() error {
		// get chatID
		if chatID, err = strconv.Atoi(c.Params("id")); err != nil {
			return BadRequest("invalid chat_id")
		}

		// read body
		if _, message, err = c.ReadMessage(); err != nil {
			return fmt.Errorf("error receive message: %s\n", err)
		}

		// unmarshal body
		var body CreateModel
		err = json.Unmarshal(message, &body)
		if err != nil {
			return fmt.Errorf("error unmarshal text: %s", err)
		}

		// get user id
		userID, err = GetUserIDFromWs(c)
		if err != nil {
			return Unauthorized()
		}

		// load chat
		var chat Chat
		err = DB.Take(&chat, chatID).Error
		if err != nil {
			return err
		}

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

			// find all records to make dialogs, without sensitive content
			var records Records
			err = DB.Find(&records, "chat_id = ? and request_sensitive <> true and response_sensitive <> true", chatID).Error
			if err != nil {
				return InternalServerError()
			}

			ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
			defer cancel()

			var (
				outputChan    = make(chan InferResponseModel, 100)
				errChan       = make(chan error)
				interruptChan = make(chan any)
				duration      float64
				outputBuilder strings.Builder
			)

			// async infer
			go InferAsync(ctx, record.Request, records, outputChan, errChan, &duration)

			// async interrupt
			go func() {
				for {
					var innerError error
					if _, message, innerError = c.ReadMessage(); innerError != nil {
						errChan <- innerError
						return
					}

					var interrupt InterruptModel
					innerError = json.Unmarshal(message, &interrupt)
					if innerError != nil {
						errChan <- innerError
						return
					}

					if interrupt.Interrupt {
						close(interruptChan)
						return
					}
				}
			}()

		MainLoop:
			for {
				select {
				case <-ctx.Done():
					return InternalServerError("infer timeout")
				case response, ok := <-outputChan:
					if !ok {
						break MainLoop
					}
					outputBuilder.WriteString(response.Output)

					// output sensitive check
					if IsSensitive(outputBuilder.String()) {
						record.ResponseSensitive = true
						err = c.WriteJSON(InferResponseModel{
							Status: -2, // sensitive
							Output: DefaultResponse,
						})
						if err != nil {
							return fmt.Errorf("write sensitive error: %v", err)
						}
					}

					err = c.WriteJSON(response)
					if err != nil {
						return fmt.Errorf("write response error: %v", err)
					}
				case err = <-errChan:
					return err
				case <-interruptChan:
					err = c.WriteJSON(InferResponseModel{Status: -1, Output: "client interrupt"})
					if err != nil {
						return fmt.Errorf("write response error: %v", err)
					}
				}
			}

			// record
			record.Response = outputBuilder.String()
			record.Duration = duration

			// infer end
			err = c.WriteJSON(InferResponseModel{Status: 0})
			if err != nil {
				return fmt.Errorf("write end status error: %v", err)
			}
		}

		// store into database
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

		// return a total record structure
		err = c.WriteJSON(record)
		if err != nil {
			return fmt.Errorf("write record error: %v", err)
		}

		return nil
	}

	err = procedure()
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
	record.Response, record.Duration, err = Infer(record.Request, records)
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

		return tx.Save(&record).Error
	})

	if err != nil {
		return err
	}

	return Serialize(c, &record)
}
