package tools

import (
	"MOSS_backend/config"
	"MOSS_backend/utils"
	"bytes"
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

/*
def clean(tmp_answer):
    tmp_answer = tmp_answer.replace('\n',' ')
    tmp_answer = tmp_answer.__repr__()
    return tmp_answer

def convert(res):
    tmp_sample = []
    id = 0

    try:
        line_dict = eval(res)
        line_dict = eval(line_dict)
    except:
        # tmp_sample.append('Error Responses.')
        pass
    if 'url' in line_dict:
        tmp_answer = 'No Results.'
        if 'snippet' in line_dict['summ']:
            tmp_answer = line_dict['summ']['snippet'].__repr__()
            # tmp_answer = clean(tmp_answer)
        elif 'title' in line_dict['summ']:
            tmp_answer = line_dict['summ']['title'] + ': ' + line_dict['summ']['answer'].__repr__()
            # tmp_answer = clean(tmp_answer)
        else:
            print ("decode error:)")
            exit(0)
        tmp_sample.append('<|{}|>: {}'.format(id, tmp_answer))
        id += 1
    elif '0' in line_dict:
        item_num = 1
        for key in line_dict:
            if item_num <= 3:
                item_num += 1
            else:
                break
            tmp_answer = line_dict[key]['summ']
            tmp_answer = clean(tmp_answer)[:400]
            tmp_sample.append('<|{}|>: {}'.format(id, tmp_answer))
            id += 1
    return tmp_sample
*/

type Map = map[string]interface{}

var searchHttpClient = http.Client{Timeout: 20 * time.Second}

func clean(tmpAnswer string) string {
	tmpAnswer = strings.ReplaceAll(tmpAnswer, "\n", " ")
	tmpAnswer = strconv.Quote(tmpAnswer)
	return tmpAnswer
}

func convert(lineDict map[string]any) (results string) {
	var tmpSample []string
	id := 0

	defer func() {
		if something := recover(); something != nil {
			log.Println(something)
			results = "None"
		}
	}()

	if _, exists := lineDict["url"]; exists {
		tmpAnswer := "No Results."
		if snippet, exists := lineDict["summ"].(Map)["snippet"]; exists {
			tmpAnswer = strconv.Quote(fmt.Sprintf("%v", snippet))
		} else if title, exists := lineDict["summ"].(Map)["title"]; exists {
			answer := lineDict["summ"].(Map)["answer"]
			tmpAnswer = fmt.Sprintf("%v: %v", title, strconv.Quote(fmt.Sprintf("%v", answer)))
		} else {
			panic("search response decode error")
		}
		tmpSample = append(tmpSample, fmt.Sprintf("<|%d|>: %s", id, tmpAnswer))
	} else if _, exists := lineDict["0"]; exists {
		for key := range lineDict {
			tmpAnswer := lineDict[key].(Map)["summ"].(string)
			tmpAnswerRune := []rune(clean(tmpAnswer))
			tmpAnswerRune = tmpAnswerRune[:utils.Min(len(tmpAnswerRune), 400)]
			tmpAnswer = string(tmpAnswerRune)
			tmpSample = append(tmpSample, fmt.Sprintf("<|%d|>: %s", id, tmpAnswer))
			if id < 3 {
				id++
			} else {
				break
			}
		}
	}
	return strings.Join(tmpSample, "\n")
}

func search(request string) string {
	data, _ := json.Marshal(map[string]any{"query": request, "topk": "3"})
	res, err := searchHttpClient.Post(config.Config.ToolsSearchUrl, "application/json", bytes.NewBuffer(data))
	if err != nil {
		utils.Logger.Error("post search error: ", zap.Error(err))
		return "None"
	}

	if res.StatusCode != 200 {
		utils.Logger.Error("post search status code error: " + strconv.Itoa(res.StatusCode))
		return "None"
	}

	var results map[string]any
	responseData, err := io.ReadAll(res.Body)
	if err != nil {
		utils.Logger.Error("post search response read error: ", zap.Error(err))
		return "None"
	}
	err = json.Unmarshal(responseData, &results)
	if err != nil {
		utils.Logger.Error("post search response unmarshal error: ", zap.Error(err))
		return "None"
	}

	return convert(results)
}
