package record

import (
	"MOSS_backend/config"
	. "MOSS_backend/models"
	. "MOSS_backend/utils"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"

	"time"

	"github.com/gofiber/websocket/v2"
	websocket_client "github.com/gorilla/websocket"
	"go.uber.org/zap"
)

func InferBizWrapper(c *websocket.Conn, chat *Chat, record *Record, user *User) (err error) {
	var (
		interruptChan    = make(chan any)   // frontend interrupt channel
		connectionClosed = new(atomic.Bool) // connection closed flag
		errChan          = make(chan error) // error transmission channel
		successChan      = make(chan any)   // success infer flag
	)
	connectionClosed.Store(false)      // initialize
	defer connectionClosed.Store(true) // if this closed, stop all goroutines
	defer func() {
		if err != nil {
			Logger.Error("err receive from biz", zap.Error(err))
			err = errors.New("会话过期啦，请新开会话")
		}
	}()

	// wait for interrupt
	go interrupt(
		c,
		interruptChan,
		connectionClosed,
	)

	// wait for infer
	go func() {
		innerErr := InferPassToBizV1(
			record,
			chat,
			user,
			&InferWsContext{
				c:                c,
				connectionClosed: connectionClosed,
			},
		)
		if innerErr != nil {
			errChan <- innerErr
		} else {
			close(successChan)
		}
	}()

	for {
		select {
		case <-interruptChan:
			return NoStatus("client interrupt")
		case err = <-errChan:
			return err
		case <-successChan:
			return nil
		}
	}
}

func InferPassToBizV1(
	record *Record,
	chat *Chat,
	_ *User,
	ctx *InferWsContext,
) (
	err error,
) {
	var (
		inferUrl     string
		configObject Config
	)

	// load config
	err = LoadConfig(&configObject)
	if err != nil {
		return err
	}
	// inferUrl = configObject.ModelConfig[0].Url
	inferUrl = configObject.ModelConfig[0].Url
	for _, modelConfig := range configObject.ModelConfig {
		if modelConfig.ID == 2 { // mars
			inferUrl = modelConfig.Url
			break
		}
	}
	firstFormattedInput := mossSpecialTokenRegexp.ReplaceAllString(record.Request, " ") // replace special token

	seqStart := true
	if chat.Count > 0 {
		seqStart = false
	}
	request := Map{
		"prompt":    firstFormattedInput,
		"seq_id":    record.ChatID,
		"seq_start": seqStart,
		"seq_end":   false,
	}

	data, _ := json.Marshal(request)
	Logger.Info("infer request", zap.ByteString("data", data))
	inferTriggerResults, err := InferRequestToBiz(data, inferUrl, ctx, record)
	if err != nil {
		return err
	}

	if ctx != nil {
		if ctx.connectionClosed.Load() {
			return interruptError
		}
	}

	// save to record
	record.Response = inferTriggerResults.Output
	record.Duration = inferTriggerResults.Duration
	record.ModelID = 2

	// end
	if ctx != nil {
		err = ctx.c.WriteJSON(InferResponseModel{Status: 0})
		if err != nil {
			return fmt.Errorf("write end status error: %v", err)
		}
	}
	return nil
}

func InferRequestToBiz(
	data []byte,
	inferUrl string,
	ctx *InferWsContext,
	_ *Record,
) (*InferTriggerResponse, error) {

	// GetResponse()
	dialer := websocket_client.Dialer{}
	//向服务器发送连接请求，websocket 统一使用 ws://，默认端口和http一样都是80
	connect, _, err := dialer.Dial(inferUrl, nil)
	if nil != err {
		Logger.Error("dial Biz server error", zap.Error(err))
		return nil, unknownError
	}
	//离开作用域关闭连接，go 的常规操作
	defer func() {
		_ = connect.Close()
	}()

	//定时向客户端发送数据
	err = connect.WriteMessage(websocket.TextMessage, data)
	if nil != err {
		Logger.Error("send message err", zap.Error(err))
		return nil, err
	}

	isEnd := false
	var nowOutput string
	startTime := time.Now()

	//启动数据读取循环，读取客户端发送来的数据
	for {
		//从 websocket 中读取数据
		//messageType 消息类型，websocket 标准
		//messageData 消息数据
		if isEnd {
			break
		}
		messageType, messageData, err := connect.ReadMessage()
		if err != nil {
			Logger.Error("read message from Biz server error", zap.Error(err))
			return nil, unknownError
		}

		switch messageType {
		case websocket.TextMessage: //文本数据
			var rsp struct {
				Code int    `json:"code"`
				Msg  string `json:"msg"`
				Data struct {
					Content string `json:"content"`
				} `json:"data"`
			}
			err = json.Unmarshal(messageData, &rsp)
			if err != nil {
				Logger.Error("unmarshal Biz server response error", zap.Error(err))
				isEnd = true
				return nil, unknownError
			}

			nowOutput = rsp.Data.Content
			// before, _, found := CutLastAny(nowOutput, ",.?!\n，。？！")
			// if !found || before == detectedOutput {
			// 	continue
			// }
			// detectedOutput = before

			_ = ctx.c.WriteJSON(InferResponseModel{
				Status: 1,
				Output: nowOutput,
				Stage:  "MOSS",
			})

			if config.Config.Debug {
				Logger.Info("infer response", zap.String("output", nowOutput))
			}
		case websocket.BinaryMessage: //二进制数据
			isEnd = true
			//log.Println("end because binary")
			//log.Println(messageData)
		case websocket.CloseMessage: //关闭
			//log.Println("end because normal")
			isEnd = true
		case websocket.PingMessage: //Ping
		case websocket.PongMessage: //Pong
		default:
		}
	}
	res := &InferTriggerResponse{
		Output:   nowOutput,
		Duration: float64(time.Since(startTime)) / 1000_000_000,
	}

	return res, nil
}

func CloseSession(inferUrl string, seqId int) (*InferTriggerResponse, error) {

	// GetResponse()
	dialer := websocket_client.Dialer{}
	//向服务器发送连接请求，websocket 统一使用 ws://，默认端口和http一样都是80
	connect, _, err := dialer.Dial(inferUrl, nil)
	if nil != err {
		Logger.Error("dial Biz server error", zap.Error(err))
		return nil, err
	}
	//离开作用域关闭连接，go 的常规操作
	defer func() {
		_ = connect.Close()
	}()

	request := Map{
		"prompt":    "",
		"seq_id":    seqId,
		"seq_start": false,
		"seq_end":   true,
	}
	data, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	//定时向客户端发送数据
	err = connect.WriteMessage(websocket.TextMessage, data)
	if nil != err {
		Logger.Error("send message to biz server err", zap.Error(err))
		return nil, err
	}

	//启动数据读取循环，读取客户端发送来的数据
	for {
		//从 websocket 中读取数据
		//messageType 消息类型，websocket 标准
		//messageData 消息数据
		messageType, messageData, err := connect.ReadMessage()
		if nil != err {
			Logger.Error("read message from Biz server error", zap.Error(err))
			break
		}

		switch messageType {
		case websocket.TextMessage: //文本数据
			var rsp struct {
				Code int    `json:"code"`
				Msg  string `json:"msg"`
				Data struct {
					Content string `json:"content"`
				} `json:"data"`
			}
			err = json.Unmarshal(messageData, &rsp)
			if nil != err {
				Logger.Error("unmarshal Biz server response error", zap.Error(err))
				return nil, err
			}

			if config.Config.Debug {
				Logger.Info("Biz server response", zap.String("data", rsp.Data.Content))
			}
			return nil, nil
		case websocket.BinaryMessage: //二进制数据
			return nil, nil
			//log.Println("end because binary")
			//log.Println(messageData)
		case websocket.CloseMessage: //关闭
			return nil, nil
			//log.Println("end because normal")
		case websocket.PingMessage: //Ping
		case websocket.PongMessage: //Pong
		default:
		}
	}

	return nil, nil
}
