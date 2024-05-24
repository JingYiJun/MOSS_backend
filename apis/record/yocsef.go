package record

import (
	. "MOSS_backend/models"
	"MOSS_backend/service"
	. "MOSS_backend/utils"
	"context"
	"errors"
	"fmt"
	"github.com/gofiber/websocket/v2"
	"go.uber.org/zap"
)

// InferYocsefAsyncAPI
// @Summary infer without login in websocket
// @Tags Websocket
// @Router /yocsef/inference [get]
// @Param json body InferenceRequest true "json"
// @Success 200 {object} InferenceResponse
func InferYocsefAsyncAPI(c *websocket.Conn) {
	var (
		err error
	)

	defer func() {
		if err != nil {
			Logger.Error(
				"client websocket return with error",
				zap.Error(err),
			)
			response := InferResponseModel{Status: -1, Output: err.Error()}
			var httpError *HttpError
			if errors.As(err, &httpError) {
				response.StatusCode = httpError.Code
			}
			_ = c.WriteJSON(response)
		}
	}()

	procedure := func() error {

		// read body
		var body InferenceRequest
		if err = c.ReadJSON(&body); err != nil {
			return fmt.Errorf("error receive message: %v", err)
		}

		if body.Request == "" {
			return BadRequest("内容不能为空")
		}

		//ctx, cancel := context.WithCancelCause(context.Background())
		//defer cancel(errors.New("procedure finished"))

		// listen to interrupt and connection close
		//go func() {
		//	defer cancel(errors.New("client connection closed or interrupt"))
		//	_, _, innerErr := c.ReadMessage()
		//	if innerErr != nil {
		//		return
		//	}
		//}()

		var record *DirectRecord
		record, err = service.InferYocsef(
			context.Background(),
			c,
			body.Request,
			body.Records,
		)
		if err != nil {
			return err
		}

		DB.Create(&record)

		_ = c.WriteJSON(record)

		return nil
	}

	err = procedure()
}
