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

type searchTask struct {
	taskModel
	results          map[string]any
	processedResults map[string]PrettySearch
}

var _ task = (*searchTask)(nil)

type PrettySearch struct {
	Url   string `json:"url"`
	Title string `json:"title"`
}

var searchHttpClient = http.Client{Timeout: 20 * time.Second}

func clean(tmpAnswer string) string {
	tmpAnswer = strings.ReplaceAll(tmpAnswer, "\n", " ")
	tmpAnswer = strconv.Quote(tmpAnswer)
	return tmpAnswer
}

func (t *searchTask) postprocess() (r *ResultModel) {
	if t.results == nil || t.err != nil {
		return NoneResultModel
	}

	var (
		dict            = t.results
		tmpSample       = make([]string, 0, 3)
		processedResult = make(map[int]PrettySearch)
		id              = 0 // counter
		title, url      string
	)

	defer func() {
		if something := recover(); something != nil {
			log.Println(something)
			r = NoneResultModel
		}
	}()

	if u, exists := t.results["url"]; exists {
		url = u.(string)
		tmpAnswer := "No Results."
		if summ, ok := dict["summ"]; ok {
			// in summary, there are two types of response
			if snippet, exists := summ.(Map)["snippet"]; exists {
				tmpAnswer = strconv.Quote(fmt.Sprintf("%v", snippet))
				if titleValue, exists := summ.(Map)["title"]; exists {
					title = titleValue.(string)
				}
			} else if titleValue, exists := summ.(Map)["title"]; exists {
				title = titleValue.(string)
				answer := summ.(Map)["answer"].(string)
				tmpAnswer = fmt.Sprintf("%v: %v", title, strconv.Quote(fmt.Sprintf("%v", answer)))
			} else {
				utils.Logger.Error("search response decode error")
				return NoneResultModel
			}
		} else {
			utils.Logger.Error("search response decode error")
			return NoneResultModel
		}

		tmpSample = append(tmpSample, fmt.Sprintf("<|%d|>: %s", t.s.searchResultsIndex, tmpAnswer))

		// save to processedResult
		processedResult[t.s.searchResultsIndex] = PrettySearch{
			Url:   url,
			Title: title,
		}
		t.s.searchResultsIndex += 1
	} else if _, exists := dict["0"]; exists {
		for _, value := range dict {
			// get title, url and answer
			if titleValue, exists := value.(Map)["title"]; exists {
				title = titleValue.(string)
			}
			if urlValue, exists := value.(Map)["url"]; exists {
				url = urlValue.(string)
			}
			tmpAnswer := value.(Map)["summ"].(string)
			tmpAnswerRune := []rune(clean(tmpAnswer))
			tmpAnswerRune = tmpAnswerRune[:utils.Min(len(tmpAnswerRune), 400)]
			tmpAnswer = string(tmpAnswerRune)
			tmpSample = append(tmpSample, fmt.Sprintf("<|%d|>: %s", t.s.searchResultsIndex, tmpAnswer))

			// save to processedResult
			processedResult[t.s.searchResultsIndex] = PrettySearch{
				Url:   url,
				Title: title,
			}
			t.s.searchResultsIndex += 1

			// to next or break
			if id < 3 { // topk
				id++
			} else {
				break
			}
		}
	}
	return &ResultModel{
		Result: strings.Join(tmpSample, "\n"),
		ExtraData: &ExtraDataModel{
			Type:    "search",
			Request: t.args,
			Data:    t.results,
		},
		ProcessedExtraData: &ExtraDataModel{
			Type:    t.action,
			Request: t.args,
			Data:    processedResult,
		},
	}
}

func (t *searchTask) request() {
	data, _ := json.Marshal(map[string]any{"query": t.args, "topk": "3"})
	res, err := searchHttpClient.Post(config.Config.ToolsSearchUrl, "application/json", bytes.NewBuffer(data))
	if err != nil {
		utils.Logger.Error("post search error: ", zap.Error(err))
		t.err = defaultError
		return
	}

	if res.StatusCode != 200 {
		utils.Logger.Error("post search status code error: " + strconv.Itoa(res.StatusCode))
		t.err = defaultError
		return
	}

	responseData, err := io.ReadAll(res.Body)
	if err != nil {
		utils.Logger.Error("post search response read error: ", zap.Error(err))
		t.err = defaultError
		return
	}
	// result processing
	var results Map
	err = json.Unmarshal(responseData, &results)
	if err != nil {
		utils.Logger.Error("post search response unmarshal error: ", zap.Error(err))
		t.err = defaultError
		return
	}

	t.results = results
}
