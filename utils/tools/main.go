package tools

import (
	"MOSS_backend/config"
	"fmt"
	"regexp"
	"strings"
)

type Map = map[string]any

var commandSplitRegexp = regexp.MustCompile(`\w+\("[\s\S][\s\S]*?"\)`)

func Execute(rawCommand string) (string, any) {
	if rawCommand == "None" || rawCommand == "none" {
		return "None", nil
	}

	commands := commandSplitRegexp.FindAllString(rawCommand, -1)

	var extraDataSlice = make([]map[string]any, 0)
	if len(commands) == 0 {
		return "None", extraDataSlice
	}
	for i := range commands {
		commands[i] = strings.Trim(commands[i], " ")
	}
	var resultsBuilder strings.Builder
	for i := range commands {
		if i > 1 {
			break
		}
		if i > 0 { // separator is '\n'
			resultsBuilder.WriteString("\n")
		}
		results, extraData := executeOnce(commands[i])
		resultsBuilder.WriteString(commands[i])
		resultsBuilder.WriteString(" =>\n")
		resultsBuilder.WriteString(results)
		if extraData != nil {
			extraDataSlice = append(extraDataSlice, extraData)
		}
	}
	if resultsBuilder.String() == "" {
		return "None", extraDataSlice
	}
	return resultsBuilder.String(), extraDataSlice
}

func executeOnce(command string) (result string, extraData map[string]any) {
	if config.Config.Debug {
		fmt.Println(command)
	}
	action, args := cutCommand(command)
	switch action {
	case "Search":
		return search(args)
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

func cutCommand(command string) (string, string) {
	before, after, found := strings.Cut(command, "(")
	if found {
		return before, strings.Trim(after, "\")")
	} else {
		return command, ""
	}
}
