package tools

import (
	"MOSS_backend/config"
	"MOSS_backend/utils"
	"bytes"
	"github.com/vmihailenco/msgpack/v5"
	"go.uber.org/zap"
	"io"
	"net/http"
	"strconv"
	"time"
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

var drawHttpClient = http.Client{Timeout: 20 * time.Second}

func draw(request string) (string, map[string]any) {
	reqBody, err := msgpack.Marshal(request)
	if err != nil {
		utils.Logger.Error("post draw(tools) prompt cannot marshal error: ", zap.Error(err))
		return "None", nil
	}
	res, err := drawHttpClient.Post(config.Config.ToolsDrawUrl, "application/x-msgpack", bytes.NewBuffer(reqBody))
	if err != nil {
		utils.Logger.Error("post draw(tools) error: ", zap.Error(err))
		return "None", nil
	}

	if res.StatusCode != 200 {
		utils.Logger.Error("post draw(tools) status code error: " + strconv.Itoa(res.StatusCode))
		return "None", nil
	}
	data, err := io.ReadAll(res.Body)
	if err != nil {
		utils.Logger.Error("post draw(tools) response body data cannot read error: ", zap.Error(err))
		return "None", nil
	}
	var resultsByte []byte
	if err = msgpack.Unmarshal(data, &resultsByte); err != nil {
		utils.Logger.Error("post draw(tools) response body data cannot Unmarshal error: ", zap.Error(err))
		return "None", nil
	}
	// sending resultsByte using json means `automatically encoding with BASE64`
	return "a picture of the given prompt has been finished", map[string]any{"type": "draw", "data": resultsByte, "request": request}
}
