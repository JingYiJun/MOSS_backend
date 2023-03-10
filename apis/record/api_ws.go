package record

import (
	"MOSS_backend/config"
	. "MOSS_backend/models"
	. "MOSS_backend/utils"
	"MOSS_backend/utils/sensitive"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gofiber/websocket/v2"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"log"
	"strconv"
	"sync/atomic"
)

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
		message []byte
		err     error
		user    *User
		banned  bool
		chat    Chat
	)

	defer func() {
		if err != nil {
			Logger.Error(
				"client websocket return with error",
				zap.Error(err),
			)
			response := InferResponseModel{Status: -1, Output: err.Error()}
			if httpError, ok := err.(*HttpError); ok {
				response.StatusCode = httpError.Code
			}
			if errors.Is(err, maxLengthExceededError) {
				chat.MaxLengthExceeded = true
				DB.Save(&chat)
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
		user, err = LoadUserFromWs(c)
		if err != nil {
			return Unauthorized()
		}

		banned, err = user.CheckUserOffense()
		if err != nil {
			return err
		}
		if banned {
			return Forbidden(OffenseMessage)
		}

		// load chat
		err = DB.Take(&chat, chatID).Error
		if err != nil {
			return err
		}

		// permission
		if chat.UserID != user.ID {
			return Forbidden()
		}

		// max length exceeded
		if chat.MaxLengthExceeded {
			return maxLengthExceededError
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
				err = c.WriteJSON(InferResponseModel{
					Status: -2, // banned
					Output: OffenseMessage,
				})
			} else {
				err = c.WriteJSON(InferResponseModel{
					Status: -2, // sensitive
					Output: DefaultResponse,
				})
			}
			if err != nil {
				return fmt.Errorf("write sensitive error: %v", err)
			}
		} else {
			/* infer */

			// find all records to make dialogs, without sensitive content
			var records Records
			err = DB.Find(&records, "chat_id = ? and request_sensitive <> true and response_sensitive <> true", chatID).Error
			if err != nil {
				return err
			}

			// async infer
			err = InferAsync(c, record.Request, records.ToRecordModel(), &record, user)
			if err != nil {
				if httpError, ok := err.(*HttpError); ok && httpError.MessageType == MaxLength {
					DB.Model(&chat).Update("max_length_exceeded", true)
				}
				return err
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

// RegenerateAsync
// @Summary regenerate a record
// @Tags Websocket
// @Router /ws/chats/{chat_id}/regenerate [get]
// @Param chat_id path int true "chat id"
// @Success 201 {object} models.Record
func RegenerateAsync(c *websocket.Conn) {
	var (
		chatID int
		user   *User
		err    error
		banned bool
		chat   Chat
	)

	defer func() {
		if err != nil {
			Logger.Error(
				"client websocket return with error",
				zap.Error(err),
			)
			response := InferResponseModel{Status: -1, Output: err.Error()}
			if httpError, ok := err.(*HttpError); ok {
				response.StatusCode = httpError.Code
			}
			if errors.Is(err, maxLengthExceededError) {
				chat.MaxLengthExceeded = true
				DB.Save(&chat)
			}
			err = c.WriteJSON(response)
			if err != nil {
				log.Println("write err error: ", err)
			}
		}
	}()

	procedure := func() error {
		// get chatID
		if chatID, err = strconv.Atoi(c.Params("id")); err != nil {
			return BadRequest("invalid chat_id")
		}

		// get user id
		user, err = LoadUserFromWs(c)
		if err != nil {
			return Unauthorized()
		}

		banned, err = user.CheckUserOffense()
		if err != nil {
			return err
		}
		if banned {
			return Forbidden(OffenseMessage)
		}

		// load chat
		err = DB.Take(&chat, chatID).Error
		if err != nil {
			return err
		}

		// permission
		if chat.UserID != user.ID {
			return Forbidden()
		}

		// max length exceeded
		if chat.MaxLengthExceeded {
			return maxLengthExceededError
		}

		// get the latest record
		var oldRecord Record
		err = DB.Last(&oldRecord, "chat_id = ?", chatID).Error
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
					err = c.WriteJSON(InferResponseModel{
						Status: -2, // banned
						Output: OffenseMessage,
					})
				} else {
					err = c.WriteJSON(InferResponseModel{
						Status: -2, // sensitive
						Output: DefaultResponse,
					})
				}
				if err != nil {
					return fmt.Errorf("write sensitive error: %v", err)
				}
			}
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

		// async infer
		err = InferAsync(c, record.Request, records.ToRecordModel(), &record, user)
		if err != nil {
			if httpError, ok := err.(*HttpError); ok && httpError.MessageType == MaxLength {
				DB.Model(&chat).Update("max_length_exceeded", true)
			}
			return err
		}

		// store into database
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

		// return a total record structure
		err = c.WriteJSON(record)
		if err != nil {
			return fmt.Errorf("write record error: %v", err)
		}

		return nil
	}

	err = procedure()
}

func interrupt(c *websocket.Conn, interruptChan chan any, connectionClosed *atomic.Bool) {
	var message []byte
	var err error
	defer connectionClosed.Store(true)
	for {
		if connectionClosed.Load() {
			return
		}
		if _, message, err = c.ReadMessage(); err != nil {
			if connectionClosed.Load() {
				return
			}
			Logger.Error("receive from client error", zap.Error(err))
			close(interruptChan)
			return
		}

		if config.Config.Debug {
			log.Printf("receive from client: %v\n", string(message))
		}

		var interruptModel InterruptModel
		err = json.Unmarshal(message, &interruptModel)
		if err != nil {
			Logger.Error("fail to unmarshal interrupt", zap.ByteString("request", message))
			continue
		}

		if interruptModel.Interrupt {
			close(interruptChan)
			return
		}
	}
}

// InferWithoutLoginAsync
// @Summary infer without login in websocket
// @Tags Websocket
// @Router /ws/inference [get]
// @Param json body InferenceRequest true "json"
// @Success 200 {object} InferenceResponse
func InferWithoutLoginAsync(c *websocket.Conn) {
	var (
		message []byte
		err     error
		record  Record
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

		// read body
		if _, message, err = c.ReadMessage(); err != nil {
			return fmt.Errorf("error receive message: %s\n", err)
		}

		// unmarshal body
		var body InferenceRequest
		err = json.Unmarshal(message, &body)
		if err != nil {
			return fmt.Errorf("error unmarshal text: %s", err)
		}

		// sensitive request check
		if sensitive.IsSensitive(body.String(), &User{}) {

			err = c.WriteJSON(InferResponseModel{
				Status: -2, // sensitive
				Output: DefaultResponse,
			})
			if err != nil {
				return fmt.Errorf("write sensitive error: %v", err)
			}
		} else {
			/* infer */

			// async infer
			err = InferAsync(c, body.Request, body.Records, &record, &User{})
			if err != nil {
				return err
			}
		}

		// store into database
		directRecord := DirectRecord{
			Records:  append(body.Records, RecordModel{Request: body.Request, Response: record.Response}),
			Duration: record.Duration,
		}
		_ = DB.Create(&directRecord).Error

		return nil
	}

	err = procedure()
}
