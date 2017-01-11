package main

import (
	"gopkg.in/natefinch/lumberjack.v2"
	"log"
	"os"
	"path/filepath"
	"time"
)

const (
	logDir, err = os.Mkdir("/var/log/netsarlacc/Logs", 664)
)

func RequestLogger(b byte) {
	fileName := time.Now().String()
	if err != nil {
		Logger(err)
	}
	logPath := filepath.Join(logDir, fileName)

	l := &lumberjack.Logger{
		Filename: logPath,
		MaxSize:  500,
		MaxAge:   31,
	}
	defer l.Close()
	log.SetOutput(l)
	logWritter, err := l.Write(b)

}
