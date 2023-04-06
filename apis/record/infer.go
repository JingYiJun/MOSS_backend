package record

import (
	"MOSS_backend/config"
	. "MOSS_backend/models"
	. "MOSS_backend/utils"
	"MOSS_backend/utils/sensitive"
	"MOSS_backend/utils/tools"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gofiber/websocket/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var endContentRegexp = regexp.MustCompile(`<[es]o\w>`)

var resultsRegexp = regexp.MustCompile(`<[|]Results[|]>[\s\S]+?<eor>`) // not greedy

var maxLengthExceededError = BadRequest("The maximum context length is exceeded").WithMessageType(MaxLength)

var unknownError = InternalServerError("unknown error, please try again")

var sensitiveError = errors.New("sensitive")

type InferResponseModel struct {
	Status     int    `json:"status"` // 1 for output, 0 for end, -1 for error, -2 for sensitive
	StatusCode int    `json:"status_code,omitempty"`
	Output     string `json:"output"`
}

type responseChannel struct {
	ch     chan InferResponseModel
	closed atomic.Bool
}

var InferResponseChannel sync.Map

var inferHttpClient = http.Client{Timeout: 1 * time.Minute}

func Infer(record *Record, prefix string) (err error) {
	var extraData any
	formattedText := InferPreprocess(record.Request, prefix)
	request := Map{"x": formattedText}

	// get params
	err = LoadParamToMap(request)
	if err != nil {
		return err
	}
	data, _ := json.Marshal(request)

	output, duration, err := inferTrigger(data)
	if err != nil {
		return err
	}

	output = strings.Trim(output, " \t\n")
	if strings.HasSuffix(output, "<eoc>") {
		// output end with <|Command|>:xxx<eoc>

		outputCommand := strings.TrimSuffix(output, "<eoc>")
		index := strings.LastIndex(output, "<|Commands|>:")
		if index == -1 {
			log.Printf("error find \"<|Commands|>:\" from inference server, output: \"%v\"\n", output)
			return InternalServerError()
		}
		outputCommand = strings.Trim(outputCommand[index+13:], " ")

		var results string
		// get results from tools
		results, extraData = tools.Execute(outputCommand)

		// generate new formatted text
		formattedText = InferWriteResult(results, output+"\n")
		request["x"] = formattedText
		data, _ = json.Marshal(request)

		output, duration, err = inferTrigger(data)
		if err != nil {
			return err
		}
	}
	// save record prefix for next inference
	record.Prefix = output
	// output end with others
	index := strings.LastIndex(record.Prefix, "<|MOSS|>:")
	if index == -1 {
		log.Printf("error find \"<|MOSS|>:\" from inference server, output: \"%v\"\n", record.Prefix)
		return InternalServerError()
	}
	record.Response = cutEndFlag(record.Prefix[index+9:])
	record.Duration = duration
	record.ExtraData = extraData

	// cut out the latest context
	humanIndex := strings.LastIndex(record.Prefix, "<|Human|>:")
	if humanIndex == -1 {
		log.Printf("error find \"<|Human|>:\" from inference server, output: \"%v\"\n", record.Prefix)
		return InternalServerError()
	}
	record.RawContent = record.Prefix[humanIndex:]
	return nil
}

func InferAsync(
	c *websocket.Conn,
	prefix string,
	record *Record,
	user *User,
) (
	err error,
) {
	var (
		interruptChan    = make(chan any)   // frontend interrupt channel
		connectionClosed = new(atomic.Bool) // connection closed flag
		errChan          = make(chan error) // error transmission channel
		successChan      = make(chan any)   // success infer flag
	)
	connectionClosed.Store(false)      // initialize
	defer connectionClosed.Store(true) // if this closed, stop all goroutines

	// wait for interrupt
	go interrupt(
		c,
		interruptChan,
		connectionClosed,
	)

	// wait for infer
	go func() {
		innerErr := inferLogicPath(
			c,
			record,
			prefix,
			user,
			connectionClosed,
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

// inferLogicPath hand out inference tasks
func inferLogicPath(
	c *websocket.Conn,
	record *Record,
	prefix string,
	user *User,
	connectionClosed *atomic.Bool,
) error {
	var (
		err       error
		innerErr  error
		request   = map[string]any{}
		uuidText  = strings.ReplaceAll(uuid.NewString(), "-", "")
		extraData any
	)

	// load params from db
	err = LoadParamToMap(request)
	if err != nil {
		return err
	}

	cleanedPrefix := resultsRegexp.ReplaceAllString(prefix, "<|Results|>: None<eor>")

	/* first infer */

	var wg1 sync.WaitGroup

	wg1.Add(1)
	// start a listener
	go func() {
		defer wg1.Done()
		innerErr = inferListener(
			c,
			record,
			user,
			uuidText,
			connectionClosed,
		)
	}()

	// get formatted text
	formattedText := InferPreprocess(record.Request, cleanedPrefix)

	// construct infer trigger data
	request["x"] = formattedText
	request["url"] = config.Config.CallbackUrl + "?uuid=" + uuidText

	// construct data to send
	data, _ := json.Marshal(request)

	// block here
	output, duration, err := inferTrigger(data)

	wg1.Wait()
	if innerErr != nil {
		return innerErr
	}

	if connectionClosed.Load() {
		return nil
	}

	output = strings.Trim(output, " \t\n")
	if strings.HasSuffix(output, "<eoc>") {
		// output ended with <|Commands|>:xxx<eoc>

		/* second infer */
		// cut out command
		outputCommand := strings.TrimSuffix(output, "<eoc>")
		index := strings.LastIndex(output, "<|Commands|>:")
		if index == -1 {
			log.Printf("error find \"<|Commands|>:\" from inference server, output: \"%v\"\n", output)
			return InternalServerError()
		}
		outputCommand = strings.Trim(outputCommand[index+13:], " ")

		var results string
		// get results from tools
		results, extraData = tools.Execute(outputCommand)

		if connectionClosed.Load() {
			return nil
		}

		// generate new formatted text and uuid
		uuidText = strings.ReplaceAll(uuid.NewString(), "-", "")
		formattedText = InferWriteResult(results, output+"\n")
		request["x"] = formattedText
		request["url"] = config.Config.CallbackUrl + "?uuid=" + uuidText
		data, _ = json.Marshal(request)

		var wg sync.WaitGroup

		wg.Add(1)
		go func() {
			defer wg.Done()
			innerErr = inferListener(
				c,
				record,
				user,
				uuidText,
				connectionClosed,
			)
		}()

		output, duration, err = inferTrigger(data)
		if err != nil {
			return err
		}

		wg.Wait()

		if innerErr != nil {
			return innerErr
		}
	}

	if connectionClosed.Load() {
		return nil
	}

	// save record prefix for next inference
	record.Prefix = output
	// output end with others
	index := strings.LastIndex(record.Prefix, "<|MOSS|>:")
	if index == -1 {
		log.Printf("error find \"<|MOSS|>:\" from inference server, output: \"%v\"\n", record.Prefix)
		return InternalServerError()
	}
	record.Response = cutEndFlag(record.Prefix[index+9:])
	record.Duration = duration
	record.ExtraData = extraData

	// cut out the latest context
	humanIndex := strings.LastIndex(record.Prefix, "<|Human|>:")
	if humanIndex == -1 {
		log.Printf("error find \"<|Human|>:\" from inference server, output: \"%v\"\n", record.Prefix)
		return InternalServerError()
	}
	record.RawContent = record.Prefix[humanIndex:]

	// end
	err = c.WriteJSON(InferResponseModel{Status: 0})
	if err != nil {
		return fmt.Errorf("write end status error: %v", err)
	}
	return nil
}

// inferListener listen from output channel
func inferListener(
	c *websocket.Conn,
	record *Record,
	user *User,
	uuidText string,
	connectionClosed *atomic.Bool,
) error {
	var err error

	// make store channel into map
	outputChan := make(chan InferResponseModel, 100)
	responseCh := &responseChannel{ch: outputChan}
	InferResponseChannel.Store(uuidText, responseCh)
	defer func() {
		InferResponseChannel.Delete(uuidText)
	}()

	startTime := time.Now()

	var timer = time.NewTimer(11 * time.Second)

	var nowOutput string
	var detectedOutput string

	for {
		if connectionClosed.Load() {
			return nil
		}
		if responseCh.closed.Load() {
			return InternalServerError()
		}
		select {
		case response := <-outputChan:
			timer.Reset(11 * time.Second)
			if config.Config.Debug {
				log.Println("receive response from output channel")
				log.Println(response)
			}
			switch response.Status {
			case 1: // ok
				if config.Config.Debug {
					log.Printf("receive response from output channal: %v\nsensitive checking\n", response.Output)
				}

				nowOutput = response.Output
				before, _, found := CutLastAny(nowOutput, ",.?!\n，。？！")
				if !found || before == detectedOutput {
					continue
				}
				detectedOutput = before

				// output sensitive check
				if sensitive.IsSensitive(detectedOutput, user) {
					record.ResponseSensitive = true
					// log new record
					record.Response = detectedOutput
					record.Duration = float64(time.Since(startTime)) / 1000_000_000
					var banned bool
					banned, err = user.AddUserOffense(UserOffenseMoss)
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

					// if sensitive, jump out and record
					return sensitiveError
				}

				err = c.WriteJSON(InferResponseModel{
					Status: 1,
					Output: detectedOutput,
				})
				if err != nil {
					return fmt.Errorf("write response error: %v", err)
				}
			case 0: // end
				if nowOutput != detectedOutput {
					if sensitive.IsSensitive(nowOutput, user) {
						record.ResponseSensitive = true
						// log new record
						record.Response = nowOutput
						record.Duration = float64(time.Since(startTime)) / 1000_000_000
						var banned bool
						banned, err = user.AddUserOffense(UserOffenseMoss)
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

						// if sensitive, jump out and record
						return sensitiveError
					}
				}
				err = c.WriteJSON(InferResponseModel{
					Status: 1,
					Output: nowOutput,
				})
				if err != nil {
					return fmt.Errorf("write response error: %v", err)
				}
				return nil
			case -1: // error
				return InternalServerError(response.Output)
			}
		case <-timer.C:
			return InternalServerError("Internal Server Timeout")
		}
	}
}

func inferTrigger(data []byte) (string, float64, error) {
	startTime := time.Now()
	rsp, err := inferHttpClient.Post(config.Config.InferenceUrl, "application/json", bytes.NewBuffer(data)) // take the ownership of data
	if err != nil {
		Logger.Error(
			"post inference error",
			zap.Error(err),
		)
		return "", 0, InternalServerError("inference server error")
	}

	defer func() {
		_ = rsp.Body.Close()
	}()

	response, err := io.ReadAll(rsp.Body)
	if err != nil {
		Logger.Error("fail to read response body", zap.Error(err))
		return "", 0, InternalServerError()
	}

	latency := int(time.Since(startTime))
	duration := float64(latency) / 1000_000_000

	if rsp.StatusCode != 200 {
		Logger.Error(
			"inference error",
			zap.Int("latency", latency),
			zap.Int("status code", rsp.StatusCode),
			zap.ByteString("body", response),
		)
		if rsp.StatusCode == 400 {
			return "", duration, unknownError
		} else if rsp.StatusCode == 560 {
			return "", duration, unknownError
		} else if rsp.StatusCode >= 500 {
			return "", duration, InternalServerError()
		} else {
			return "", duration, unknownError
		}
	} else {
		var responseStruct struct {
			Pred                   string `json:"pred"`
			InputTokenNum          int    `json:"input_token_num"`
			NewGenerationsTokenNum int    `json:"new_generations_token_num"`
		}
		err = json.Unmarshal(response, &responseStruct)
		if err != nil {
			responseString := string(response)
			if responseString == "400" {
				return "", duration, maxLengthExceededError
			} else if responseString == "560" {
				return "", duration, unknownError
			} else {
				Logger.Error(
					"unable to unmarshal response from infer",
					zap.ByteString("response", response),
					zap.Error(err),
				)
			}
			return "", duration, InternalServerError()
		} else {
			Logger.Info(
				"inference success",
				zap.Int("latency", latency),
				zap.String("pred", responseStruct.Pred),
				zap.Int("input_token_num", responseStruct.InputTokenNum),
				zap.Int("new_generations_token_num", responseStruct.NewGenerationsTokenNum),
				zap.Float64("average", float64(latency)/float64(responseStruct.NewGenerationsTokenNum)),
			)
			return responseStruct.Pred, duration, nil
		}
	}
}

func ReceiveInferResponse(c *websocket.Conn) {
	var (
		message []byte
		err     error
	)

	defer func() {
		if something := recover(); something != nil {
			Logger.Error("receive infer response panicked", zap.Any("error", something))
		}
	}()

	uuidText := c.Query("uuid")
	if uuidText == "" {
		_ = c.WriteJSON(InferResponseModel{Status: -1, StatusCode: 400, Output: "Bad Request"})
		return
	}

	value, ok := InferResponseChannel.Load(uuidText)
	if !ok {
		Logger.Error("receive from infer invalid uuid", zap.String("uuid", uuidText))
		_ = c.WriteJSON(InferResponseModel{Status: -1, StatusCode: 400, Output: "Bad Request"})
		return
	}
	ch := value.(*responseChannel)

	for {
		if _, message, err = c.ReadMessage(); err != nil {
			if ch.closed.Load() {
				_ = c.WriteJSON(InferResponseModel{Status: 0})
				return
			} else {
				Logger.Error("receive from infer error", zap.Error(err))
				ch.closed.Store(true)
			}
			return
		}
		if ch.closed.Load() {
			_ = c.WriteJSON(InferResponseModel{Status: 0})
			return
		}

		if config.Config.Debug {
			log.Printf("receive message from inference, uuid: %v: %s\n", uuidText, string(message))
		}

		var inferResponse InferResponseModel
		err = json.Unmarshal(message, &inferResponse)
		if err != nil {
			log.Printf("receive from infer error message type: %s\n, error: %s", string(message), err)
			continue
		}

		// continue if sending a heartbeat package
		if inferResponse.Status == 2 {
			continue
		}

		// post process
		inferResponse.Output = InferPostprocess(inferResponse.Output)

		if config.Config.Debug {
			log.Printf("recieve output: %v\n", inferResponse.Output)
		}

		// may panic
		ch.ch <- inferResponse

		if inferResponse.Status == 0 {
			_ = c.WriteJSON(InferResponseModel{Status: 0})
			return
		}
	}
}

func InferPreprocess(input, prefix string) (formattedText string) {
	return prefix + fmt.Sprintf("<|Human|>: %s<eoh>\n", input)
}

func InferWriteResult(results, prefix string) string {
	return prefix + fmt.Sprintf("<|Results|>: %s<eor>\n", results)
}

func InferPostprocess(output string) (tidyOutput string) {
	// process end with 0xfffd
	runeSlice := []rune(output)
	for len(runeSlice) > 0 && runeSlice[len(runeSlice)-1] == 0xfffd {
		runeSlice = runeSlice[:len(runeSlice)-1]
	}

	// not cut
	return output
}

func cutEndFlag(content string) string {
	content = strings.Trim(content, " \n\t")
	loc := endContentRegexp.FindIndex([]byte(content))
	if loc != nil {
		content = content[:loc[0]]
	}
	return strings.Trim(content, " ")
}
