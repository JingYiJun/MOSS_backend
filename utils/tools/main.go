package tools

import (
	"MOSS_backend/config"
	"fmt"
	"strings"
	"sync"
)

func Execute(command string) (results string) {
	if command == "None" || command == "none" {
		return "None"
	}

	commands := strings.Split(command, ",")
	for i := range commands {
		strings.Trim(commands[i], " ")
	}

	var resultsBuilder struct {
		strings.Builder
		sync.Mutex
	}

	if len(commands) == 1 {
		_, _ = resultsBuilder.WriteString(commands[0])
		_, _ = resultsBuilder.WriteString(executeOnce(commands[0]))
	} else {

		// search concurrently
		var wg sync.WaitGroup

		for i := range commands {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				resultsBuilder.Lock()
				_, _ = resultsBuilder.WriteString(commands[i] + "=>\n" + executeOnce(commands[i]))
				resultsBuilder.Unlock()
			}(i)
		}

		wg.Wait()
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
