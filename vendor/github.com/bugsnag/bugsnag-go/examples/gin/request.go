package main

import (
	"github.com/bugsnag/bugsnag-go"
	"github.com/gin-gonic/gin"
	"net/http"
	"os"
)

func ConfigureRoutes(g *gin.Engine) {
	g.GET("/crash", performUnhandledCrash)
	g.GET("/handled", performHandledCrash)

}

func performUnhandledCrash(c *gin.Context) {
	c.String(http.StatusOK, "OK")
	var a struct{}
	crash(a)
}

func performHandledCrash(c *gin.Context) {
	_, err := os.Open("some_nonexistent_file.txt")
	if err != nil {
		bugsnag.Notify(err)
	}
	c.String(http.StatusOK, "OK")
}

func crash(a interface{}) string {
	return a.(string)
}
