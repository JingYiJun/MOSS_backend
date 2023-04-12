package tools

import (
	"MOSS_backend/config"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type Map = map[string]any

const maxCommandNumber = 4

var commandsFormatRegexp = regexp.MustCompile(`(Search|Solve|Calculate|Draw)\("([\s\S]+?)"\)(, *?(Search|Solve|Calculate|Draw)\("([\s\S]+?)"\))*`)
var commandSplitRegexp = regexp.MustCompile(`(Search|Solve|Calculate|Draw)\("([\s\S]+?)"\)`)
var commandOrder = map[string]int{"Search": 1, "Calculate": 2, "Solve": 3, "Draw": 4}
var CommandsFormatError = errors.New("commands format error")

func Execute(rawCommand string) (string, any, error) {
	if !config.Config.EnableTools || rawCommand == "None" || rawCommand == "none" {
		return "None", nil, nil
	}
	if !commandsFormatRegexp.MatchString(rawCommand) {
		return "None", nil, CommandsFormatError
	}
	// commands is like: [[Search("A"), Search, A,] [Solve("B"), Solve, B] [Search("C"), Search, C]]
	commands := commandSplitRegexp.FindAllStringSubmatch(rawCommand, -1)

	var extraDataSlice = make([]map[string]any, 0)
	if len(commands) == 0 {
		return "None", extraDataSlice, nil
	}
	// sort, search should be at first
	sort.Slice(commands, func(i, j int) bool {
		return commandOrder[commands[i][1]] < commandOrder[commands[j][1]]
	})
	// commands now like: [[Search("A"), Search, A,] [Search("C"), Search, C] [Solve("B"), Solve, B]]
	var resultsBuilder strings.Builder
	// the index of `the search results in <|results|>` starts with 1
	searchResultsIndex := 1
	for i := range commands {
		if i >= maxCommandNumber {
			break
		}
		if i > 0 { // separator is '\n'
			resultsBuilder.WriteString("\n")
		}
		results, extraData := executeOnce(commands[i][1], commands[i][2], &searchResultsIndex)
		resultsBuilder.WriteString(commands[i][0])
		resultsBuilder.WriteString(" =>\n")
		resultsBuilder.WriteString(results)
		if extraData != nil {
			extraDataSlice = append(extraDataSlice, extraData)
		}
	}
	if resultsBuilder.String() == "" {
		return "None", extraDataSlice, nil
	}
	return resultsBuilder.String(), extraDataSlice, nil
}

func executeOnce(action string, args string, searchResultIndex *int) (string, map[string]any) {
	if config.Config.Debug {
		fmt.Println(action + args)
	}
	switch action {
	case "Search":
		results, extraData := search(args)
		searchResult := searchResultsFormatter(results, searchResultIndex)
		return searchResult, extraData
	case "Calculate":
		return calculate(args)
	case "Solve":
		return solve(args)
	case "Draw":
		return draw(args)
	default:
		return "None", nil
	}
}

//func cutCommand(command string) (string, string) {
//	before, after, found := strings.Cut(command, "(")
//	if found {
//		return before, strings.Trim(after, "\")")
//	} else {
//		return command, ""
//	}
//}
