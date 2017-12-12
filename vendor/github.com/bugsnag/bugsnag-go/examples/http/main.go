package main

import (
	"github.com/bugsnag/bugsnag-go"
	"log"
	"net/http"
)

func main() {
	http.HandleFunc("/", Get)

	// Insert your API key
	bugsnag.Configure(bugsnag.Configuration{
		APIKey: "YOUR-API-KEY-HERE",
	})

	log.Println("Serving on 9001")
	http.ListenAndServe(":9001", bugsnag.Handler(nil))
}

func Get(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)
	w.Write([]byte("OK\n"))

	var a struct{}
	crash(a)
}

func crash(a interface{}) string {
	return a.(string)
}
