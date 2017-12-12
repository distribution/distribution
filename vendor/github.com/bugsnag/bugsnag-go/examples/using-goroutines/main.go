package main

import (
	"github.com/bugsnag/bugsnag-go"
	"sync"
)

func main() {
	// Initialize Bugsnag with your API key
	bugsnag.Configure(bugsnag.Configuration{
		APIKey: "YOUR-API-KEY-HERE",
	})
	runProcesses()
}

func runProcesses() {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// AutoNotify captures any panics, repanicking after error reports
		// are sent
		defer bugsnag.AutoNotify()

		var object struct{}
		crash(object)
	}()

	wg.Wait()
}

func crash(a interface{}) string {
	return a.(string)
}
