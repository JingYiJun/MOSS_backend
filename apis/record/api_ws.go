package record

import (
	"MOSS_backend/config"
	. "MOSS_backend/models"
	. "MOSS_backend/utils"
	"context"
	"encoding/json"
	"fmt"
	"github.com/gofiber/websocket/v2"
	"gorm.io/gorm"
	"log"
	"strconv"
	"time"
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

			err = c.WriteJSON(InferResponseModel{
				Status: -2, // sensitive
				Output: DefaultResponse,
			})
			if err != nil {
				return fmt.Errorf("write sensitive error: %v", err)
			}
		} else {
			/* infer */

			// find all records to make dialogs, without sensitive content
			var records Records
			err = DB.Find(&records, "chat_id = ? and request_sensitive <> true and response_sensitive <> true", chatID).Error
			if err != nil {
				return InternalServerError()
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			var (
				outputChan    = make(chan InferResponseModel, 100)
				errChan       = make(chan error)
				interruptChan = make(chan any)
				duration      float64
				lastResponse  InferResponseModel
			)

			// async infer
			go InferAsync(ctx, record.Request, records, outputChan, errChan, &duration)

			// async interrupt & heart beat
			go func() {
				for {
					var innerError error
					if _, message, innerError = c.ReadMessage(); innerError != nil {
						errChan <- innerError
						return
					}

					if config.Config.Debug {
						log.Printf("receive from client: %v\n", string(message))
					}

					var interrupt InterruptModel
					innerError = json.Unmarshal(message, &interrupt)
					if innerError != nil {
						log.Printf("error unmarshal interrupt: %v\n", string(message))
						continue
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

					if config.Config.Debug {
						log.Printf("receive response from output channal: %v\nsensitive checking\n", response.Output)
					}

					if len(response.Output) == len(lastResponse.Output) {
						continue
					}
					lastResponse = response

					// output sensitive check
					if IsSensitive(response.Output) {
						record.ResponseSensitive = true
						err = c.WriteJSON(InferResponseModel{
							Status: -2, // sensitive
							Output: DefaultResponse,
						})
						if err != nil {
							return fmt.Errorf("write sensitive error: %v", err)
						}
					}

					if config.Config.Debug {
						log.Printf("not sensitive")
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
					return nil
				}
			}

			// record
			record.Response = lastResponse.Output
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

// RegenerateAsync
// @Summary regenerate a record
// @Tags Websocket
// @Router /ws/chats/{chat_id}/regenerate [get]
// @Param chat_id path int true "chat id"
// @Success 201 {object} models.Record
func RegenerateAsync(c *websocket.Conn) {
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

		// get the latest record
		var oldRecord Record
		err = DB.Last(&oldRecord, "chat_id = ?", chatID).Error
		if err != nil {
			return err
		}

		if oldRecord.RequestSensitive {
			err = c.WriteJSON(InferResponseModel{
				Status: -2, // sensitive
				Output: DefaultResponse,
			})
			if err != nil {
				return fmt.Errorf("write sensitive error: %v", err)
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
			return InternalServerError()
		}

		// remove the latest record
		if len(records) > 0 {
			records = records[0 : len(records)-1]
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		var (
			outputChan    = make(chan InferResponseModel, 100)
			errChan       = make(chan error)
			interruptChan = make(chan any)
			duration      float64
			lastResponse  InferResponseModel
		)

		// async infer
		go InferAsync(ctx, record.Request, records, outputChan, errChan, &duration)

		// async interrupt & heart beat
		go func() {
			for {
				var innerError error
				if _, message, innerError = c.ReadMessage(); innerError != nil {
					errChan <- innerError
					return
				}

				if config.Config.Debug {
					log.Printf("receive from client: %v\n", string(message))
				}

				var interrupt InterruptModel
				innerError = json.Unmarshal(message, &interrupt)
				if innerError != nil {
					log.Printf("error unmarshal interrupt: %v\n", string(message))
					continue
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

				if config.Config.Debug {
					log.Printf("receive response from output channal: %v\nsensitive checking\n", response.Output)
				}

				if len(response.Output) == len(lastResponse.Output) {
					continue
				}
				lastResponse = response

				// output sensitive check
				if IsSensitive(response.Output) {
					record.ResponseSensitive = true
					err = c.WriteJSON(InferResponseModel{
						Status: -2, // sensitive
						Output: DefaultResponse,
					})
					if err != nil {
						return fmt.Errorf("write sensitive error: %v", err)
					}
				}

				if config.Config.Debug {
					log.Printf("not sensitive")
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
				return nil
			}
		}

		// record
		record.Response = lastResponse.Output
		record.Duration = duration

		// infer end
		err = c.WriteJSON(InferResponseModel{Status: 0})
		if err != nil {
			return fmt.Errorf("write end status error: %v", err)
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
