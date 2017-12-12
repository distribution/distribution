package controllers

import (
	"github.com/revel/revel"
)

type Application struct {
	*revel.Controller
}

func (c Application) Index() revel.Result {
	var a struct{}
	crash(a)

	return c.Render()
}

func crash(a interface{}) string {
	return a.(string)
}
