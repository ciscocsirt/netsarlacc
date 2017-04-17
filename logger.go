package main

import (
        "gopkg.in/natefinch/lumberjack.v2"
        "log"
        "log/syslog"
        "fmt"
        "os"
        "io"
        "time"
)

func AppLogger(err error) {
        logwriter, e := syslog.New(syslog.LOG_CRIT, "netsarlacc")
        if e == nil {
                log.SetOutput(logwriter)
        }
        log.Print(err)
}

func writeLogger(Logchan chan string) {
        //variables
        var logFile *os.File
        var writeFile bool = true
	var err error

        //ticker and file rotation goroutine
        ticker := time.NewTicker(time.Minute * 10)
        go func() {
        	//get current datetime and creates filename based on current time
        	now := time.Now()
        	filename := ("sinkhole-" + now.Format("2006-01-02-15-04-05") + ".log")

        	//create inital file
        	logFile, err = os.OpenFile(filename, os.O_CREATE|os.O_RDWR, 0666)
        	if err != nil {
                	log.Fatal(err)
       		}
                for range ticker.C {
                        writeFile = false
                        logFile.Close()
			//get current datetime and creates filename based on current time
        		now := time.Now()
                        filename := ("sinkhole-" + now.Format("2006-01-02-15-04-05") + ".log")
                        logFile, err = os.OpenFile(filename, os.O_CREATE|os.O_RDWR, 0666)
                        if err != nil {
                                log.Fatal(err)
                        }
                        writeFile = true
                }
        }()


        defer logFile.Close()

        for l := range Logchan {
                if writeFile == true{
                        n, err := io.WriteString(logFile, l + "\n")
                        if err != nil {
                                fmt.Println(n, err)
                        }
                }
        }
}
