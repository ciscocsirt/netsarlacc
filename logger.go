package main

import (
	"log"
	"log/syslog"
)

func Logger(err error) {
	logwriter, e := syslog.New(syslog.LOG_CRIT, "netsarlacc")
	if e == nil {
		log.SetOutput(logwriter)
	}
	log.Print(err)
}
