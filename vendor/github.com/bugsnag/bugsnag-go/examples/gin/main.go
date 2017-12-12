package main

import (
	"github.com/bugsnag/bugsnag-go"
	"github.com/bugsnag/bugsnag-go/gin"
	"github.com/gin-gonic/gin"
)

func main() {

	g := gin.Default()

	// Insert your API key
	g.Use(bugsnaggin.AutoNotify(bugsnag.Configuration{
		APIKey:          "YOUR-API-KEY-HERE",
		ProjectPackages: []string{"main", "github.com/bugsnag/bugsnag-go/examples/gin"},
	}))

	ConfigureRoutes(g)

	g.Run(":9001") // listen and serve on 0.0.0.0:9001
}
