package tools

import (
	"MOSS_backend/config"
	"fmt"
	"regexp"
	"strings"
)

type Map = map[string]any

var commandRegexp = regexp.MustCompile(`\w+\("[\s\S]+"\)`)

func Execute(rawCommand string) (string, any) {
	if rawCommand == "None" || rawCommand == "none" {
		return "None", nil
	}

	commands := strings.Split(rawCommand, ",")
	for i := range commands {
		commands[i] = strings.Trim(commands[i], " ")
	}

	var resultsBuilder strings.Builder
	var extraDataSlice = make([]map[string]any, 0)

	if len(commands) == 1 {
		command := commands[0]
		if commandRegexp.MatchString(command) {
			results, extraData := executeOnce(commands[0])
			_, _ = resultsBuilder.WriteString(command)
			_, _ = resultsBuilder.WriteString(" =>\n")
			_, _ = resultsBuilder.WriteString(results)
			if extraData != nil {
				extraDataSlice = append(extraDataSlice, extraData)
			}
		} else {
			return "None", extraDataSlice
		}
	} else {
		for i := range commands {
			if i > 1 {
				break
			}
			if commandRegexp.MatchString(commands[i]) {
				results, extraData := executeOnce(commands[i])
				_, _ = resultsBuilder.WriteString(commands[i] + "=>\n" + results + "\n")
				if extraData != nil {
					extraDataSlice = append(extraDataSlice, extraData)
				}
			}
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
	//case "Draw":
	//	return draw(args)
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
