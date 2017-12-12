package main

import (
	"github.com/bugsnag/bugsnag-go"
	"github.com/bugsnag/bugsnag-go/negroni"
	"github.com/urfave/negroni"
	"log"
	"net/http"
	"os"
)

func main() {
	errorReporterConfig := bugsnag.Configuration{
		APIKey: "YOUR API KEY",
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("OK\n"))

		var a struct{}
		crash(a)
	})
	mux.HandleFunc("/handled", func(w http.ResponseWriter, req *http.Request) {
		_, err := os.Open("some_nonexistent_file.txt")
		if err != nil {
			bugsnag.Notify(err, errorReporterConfig)
		}
		w.WriteHeader(200)
		w.Write([]byte("OK\n"))
	})

	n := negroni.New()
	n.Use(negroni.NewRecovery())
	n.Use(bugsnagnegroni.AutoNotify(errorReporterConfig))
	n.UseHandler(mux)

	log.Println("Serving on 9001")
	http.ListenAndServe(":9001", n)
}

func crash(a interface{}) string {
	return a.(string)
}
