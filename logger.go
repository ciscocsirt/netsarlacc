package main

import (
	"gopkg.in/natefinch/lumberjack.v2"
	"log"
	"log/syslog"
	"fmt"
)

func AppLogger(err error) {
	logwriter, e := syslog.New(syslog.LOG_CRIT, "netsarlacc")
	if e == nil {
		log.SetOutput(logwriter)
	}
	log.Print(err)
}

func ConnLogger(v interface{}) {
	log.SetOutput(&lumberjack.Logger{
		Filename: "netsarlacc.log",
		MaxSize:  1, // megabytes
	})
	log.Println(v)
}

func writeLogger(logChan chan string) {
	s := <-logChan
	fmt.Println(s)
}
	
