package tools

import (
	"MOSS_backend/config"
	"MOSS_backend/utils"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/gofiber/websocket/v2"
	"go.uber.org/zap"
)

type Map = map[string]any
type CommandStatusModel struct {
	Status int    `json:"status"`
	ID     int    `json:"id"`
	Args   string `json:"output"`
	Type   string `json:"type"`
	Stage  string `json:"stage"`
}

const maxCommandNumber = 4

var commandsFormatRegexp = regexp.MustCompile(`\w+\("([\s\S]+?)"\)(, *?\w+\("([\s\S]+?)"\))*`)
var commandSplitRegexp = regexp.MustCompile(`(\w+)\("([\s\S]+?)"\)`)
var commandOrder = map[string]int{"Search": 1, "Calculate": 2, "Solve": 3, "Text2Image": 4}
var Command2Description = map[string]string{"Search": "Web search", "Calculate": "Calculator", "Solve": "Equation solver", "Text2Image": "Text-to-image" }
var ErrInvalidCommandFormat = errors.New("commands format error")
var ErrCommandIsNotNone = errors.New("command is not none")

func Execute(c *websocket.Conn, rawCommand string, pluginConfig map[string]bool) (*ResultTotalModel, string, error) {
	if rawCommand == "None" || rawCommand == "none" {
		return NoneResultTotalModel, "None", ErrCommandIsNotNone
	}
	if !config.Config.EnableTools {
		return NoneResultTotalModel, "None", ErrCommandIsNotNone
	}
	if command := commandsFormatRegexp.FindString(rawCommand); command != rawCommand {
		return NoneResultTotalModel, "None", ErrInvalidCommandFormat
	}
	// commands is like: [[Search("A"), Search, A,] [Solve("B"), Solve, B] [Search("C"), Search, C]]
	commands := commandSplitRegexp.FindAllStringSubmatch(rawCommand, -1)

	commands, newCommandString, err := filterCommand(commands, pluginConfig)
	if err != nil {
		return NoneResultTotalModel, "None", err
	}

	// sort, search should be at first
	sort.Slice(commands, func(i, j int) bool {
		return commandOrder[commands[i][1]] < commandOrder[commands[j][1]]
	})
	// commands now like: [[Search("A"), Search, A,] [Search("C"), Search, C] [Solve("B"), Solve, B]]

	var s = &scheduler{
		tasks: make([]task, 0, len(commands)),
		// the index of `the search results in <|results|>` starts with 1
		searchResultsIndex: 1,
	}

	var resultTotal = &ResultTotalModel{
		ExtraData:          make([]*ExtraDataModel, 0, len(commands)),
		ProcessedExtraData: make([]*ExtraDataModel, 0, len(commands)),
	}

	// generate tasks
	for i := range commands {
		if i >= maxCommandNumber {
			break
		}
		sendCommandStatus(c, i, commands[i][1], commands[i][2], "start")
		t := s.NewTask(commands[i][1], commands[i][2])
		if t != nil {
			s.tasks = append(s.tasks, t)
		}
	}

	// request tools concurrently
	var wg sync.WaitGroup
	for _, t := range s.tasks {
		wg.Add(1)
		go func(t task) {
			defer wg.Done()
			t.request()
		}(t)
	}
	wg.Wait()

	// postprocess
	var resultsBuilder strings.Builder
	for i, t := range s.tasks {
		results := t.postprocess()

		if i > 0 { // separator is '\n'
			resultsBuilder.WriteString("\n")
		}
		resultsBuilder.WriteString(t.name())
		resultsBuilder.WriteString(" =>\n")
		resultsBuilder.WriteString(results.Result)
		if results.ExtraData != nil {
			resultTotal.ExtraData = append(resultTotal.ExtraData, results.ExtraData)
		}
		if results.ProcessedExtraData != nil {
			resultTotal.ProcessedExtraData = append(resultTotal.ProcessedExtraData, results.ProcessedExtraData)
		}
		sendCommandStatus(c, i, commands[i][1], commands[i][2], "done")
	}

	if resultsBuilder.String() == "" {
		return NoneResultTotalModel, "None", nil
	}

	resultTotal.Result = resultsBuilder.String()
	return resultTotal, newCommandString, nil
}

func (s *scheduler) NewTask(action string, args string) task {
	if config.Config.Debug {
		fmt.Println(action + args)
	}
	t := taskModel{
		s:      s,
		action: action,
		args:   args,
		err:    nil,
	}
	switch action {
	case "Search":
		return &searchTask{taskModel: t}
	case "Calculate":
		return &calculateTask{taskModel: t}
	case "Solve":
		return &solveTask{taskModel: t}
	case "Text2Image":
		return &drawTask{taskModel: t}
	default:
		return nil
	}
}

// sendCommandStatus
// a filter. only inform frontend well-formed commands
func sendCommandStatus(c *websocket.Conn, id int, action, args, StatusString string) {
	if c == nil {
		utils.Logger.Info("no ws connection")
		return
	}
	if err := c.WriteJSON(CommandStatusModel{
		Status: 3,      // 3 means `send command status`
		ID:     id + 1, // id start with 1
		Type:   action,
		Args:   args,
		Stage:  StatusString, // start or done
	}); err != nil {
		utils.Logger.Error("fail to send command status", zap.Error(err))
	}
}

func filterCommand(commands [][]string, pluginConfig map[string]bool) ([][]string, string, error) {
	var newCommandBuilder strings.Builder
	var validCommands = make([][]string, 0, len(commands))
	for i := range commands {
		if description, ok := Command2Description[commands[i][1]]; !ok {
			continue
		} else {
			if v, ok := pluginConfig[description]; !ok || !v {
				continue
			}
		}
		validCommands = append(validCommands, commands[i])
		if i > 0 {
			newCommandBuilder.WriteString(", ")
		}
		newCommandBuilder.WriteString(commands[i][0])
	}
	if len(validCommands) == 0 {
		return nil, "None", ErrCommandIsNotNone
	}
	return validCommands, newCommandBuilder.String(), nil
}

//func executeOnce(action string, args string, searchResultIndex *int) (string, map[string]any) {
//	if config.Config.Debug {
//		fmt.Println(action + args)
//	}
//	switch action {
//	case "Search":
//		results, extraData := search(args)
//		searchResult := searchResultsFormatter(results, searchResultIndex)
//		return searchResult, extraData
//	case "Calculate":
//		return calculate(args)
//	case "Solve":
//		return solve(args)
//	case "Draw":
//		return draw(args)
//	default:
//		return "None", nil
//	}
//}

//func cutCommand(command string) (string, string) {
//	before, after, found := strings.Cut(command, "(")
//	if found {
//		return before, strings.Trim(after, "\")")
//	} else {
//		return command, ""
//	}
//}
