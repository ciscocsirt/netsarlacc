package main

import (
	"log"
	"log/syslog"
)

func Logger(err error) {
	// TODO: implment using syslog writer
	// logWriter, err := syslog.Dial("udp", "localhost", syslog.LOG_ERR, "Error Logger")
	// defer logWriter.Close()
	// if err != nil {
	// 	log.Fatal("Error")
	// }

	logwriter, e := syslog.New(syslog.LOG_NOTICE, "netsarlacc")
	if e == nil {
		log.SetOutput(logwriter)
	}
	log.Print(err)
}
