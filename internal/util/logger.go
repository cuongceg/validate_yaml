package util

import (
	"io"
	"log"
	"os"
)

var App *log.Logger

func Init() {
	f, _ := os.OpenFile("app.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	mw := io.MultiWriter(os.Stdout, f)
	App = log.New(mw, "APP: ", log.Ldate|log.Ltime|log.Lshortfile)
}
