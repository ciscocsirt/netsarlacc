package main

import (
	"log"
	"log/syslog"
)

func Logger(err error) {
	// TODO:
	// -- Rotational log
	// -- write to file
	// -- takes JSON
	logwriter, e := syslog.New(syslog.LOG_NOTICE, "netsarlacc")
	if e == nil {
		log.SetOutput(logwriter)
	}
	log.Print(err)
}
