package tools

import (
	"MOSS_backend/config"
	"fmt"
	"regexp"
	"strings"
)

var commandRegexp = regexp.MustCompile(`\w+\(".+"\)`)

func Execute(rawCommand string) string {
	if rawCommand == "None" || rawCommand == "none" {
		return "None"
	}

	commands := strings.Split(rawCommand, ",")
	for i := range commands {
		commands[i] = strings.Trim(commands[i], " ")
	}

	var resultsBuilder strings.Builder

	if len(commands) == 1 {
		command := commands[0]
		if commandRegexp.MatchString(command) {
			_, _ = resultsBuilder.WriteString(command)
			_, _ = resultsBuilder.WriteString(" =>\n")
			_, _ = resultsBuilder.WriteString(executeOnce(commands[0]))
		} else {
			return "None"
		}
	} else {
		for i := range commands {
			if i > 1 {
				break
			}
			if commandRegexp.MatchString(commands[i]) {
				results := executeOnce(commands[i])
				_, _ = resultsBuilder.WriteString(commands[i] + "=>\n" + results + "\n")
			}
		}
	}
	if resultsBuilder.String() == "" {
		return "None"
	}
	return resultsBuilder.String()
}

func executeOnce(command string) (result string) {
	if config.Config.Debug {
		fmt.Println(command)
	}
	action, args := cutCommand(command)
	switch action {
	case "Search":
		return search(args)
	default:
		return "None"
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
