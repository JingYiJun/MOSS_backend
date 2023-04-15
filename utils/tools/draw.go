package tools

import (
	"MOSS_backend/config"
	"MOSS_backend/utils"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/vmihailenco/msgpack/v5"
	"go.uber.org/zap"
)

//func main() {
//	prompt := flag.String("p", "a photo of an astronaut riding a horse on Mars", "prompt")
//	host := flag.String("host", "0.0.0.0", "remote server host ip")
//	port := flag.Int("port", 443, "service port")
//	flag.Parse()
//
//	client := &http.Client{}
//	reqBody := msgpack.MustMarshal(*prompt)
//	resp, err := client.Post(
//		fmt.Sprintf("http://%s:%d", *host, *port),
//		"application/x-msgpack",
//		bytes.NewBuffer(reqBody),
//	)
//	if err != nil {
//		fmt.Printf("ERROR: %v\n", err)
//		return
//	}
//	defer resp.Body.Close()
//
//	if resp.StatusCode == http.StatusOK {
//		data := make([]byte, resp.ContentLength)
//		if _, err := resp.Body.Read(data); err != nil {
//			fmt.Printf("ERROR: %v\n", err)
//			return
//		}
//		fmt.Println(base64.StdEncoding.EncodeToString(data))
//	} else {
//		fmt.Printf("ERROR: <%d> %s\n", resp.StatusCode, resp.Status)
//	}
//}

type drawTask struct {
	taskModel
	results []byte
	url     string
}

var _ task = (*drawTask)(nil)

func (t *drawTask) request() {
	reqBody, err := msgpack.Marshal(t.args)
	if err != nil {
		utils.Logger.Error("post draw(tools) prompt cannot marshal error: ", zap.Error(err))
		t.err = ErrGeneric
		return
	}
	res, err := drawHttpClient.Post(config.Config.ToolsDrawUrl, "application/x-msgpack", bytes.NewBuffer(reqBody))
	if err != nil {
		utils.Logger.Error("post draw(tools) error: ", zap.Error(err))
		t.err = ErrGeneric
		return
	}

	if res.StatusCode != 200 {
		utils.Logger.Error("post draw(tools) status code error: " + strconv.Itoa(res.StatusCode))
		t.err = ErrGeneric
		return
	}
	data, err := io.ReadAll(res.Body)
	if err != nil {
		utils.Logger.Error("post draw(tools) response body data cannot read error: ", zap.Error(err))
		t.err = ErrGeneric
		return
	}
	var resultsByte []byte
	if err = msgpack.Unmarshal(data, &resultsByte); err != nil {
		utils.Logger.Error("post draw(tools) response body data cannot Unmarshal error: ", zap.Error(err))
		t.err = ErrGeneric
		return
	}

	// save
	t.results = resultsByte
}

func (t *drawTask) postprocess() *ResultModel {
	if t.err != nil {
		return NoneResultModel
	}
	// save to file
	filename := uuid.NewString() + ".jpg"
	err := os.WriteFile(fmt.Sprintf("./draw/%s", filename), t.results, 0644)
	if err != nil {
		utils.Logger.Error("post draw(tools) response body data cannot save to file error: ", zap.Error(err))
		return NoneResultModel
	}

	t.url = fmt.Sprintf("https://%s/api/draw/%s", config.Config.Hostname, filename)

	return &ResultModel{
		Result: "a picture of the given prompt has been finished",
		ExtraData: &ExtraDataModel{
			Type:    "draw",
			Request: t.args,
			// sending resultsByte using json means `automatically encoding with BASE64`
			Data: t.results,
		},
		ProcessedExtraData: &ExtraDataModel{
			Type:    t.action,
			Request: t.args,
			Data:    t.url,
		},
	}
}

var drawHttpClient = http.Client{Timeout: 20 * time.Second}
