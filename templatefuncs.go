package main

import (
	"fmt"
	htpl "html/template"
)

var templateFuncs = htpl.FuncMap{
	"safeJS": SafeJS,
}

func SafeJS(s interface{}) htpl.JS {
	return htpl.JS(fmt.Sprint(s))
}