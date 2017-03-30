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

func ConnLogger(v interface{}) {
        log.SetOutput(&lumberjack.Logger{
                Filename: "netsarlacc.log",
                MaxSize:  1, // megabytes
        })
        log.Println(v)
}

func writeLogger(Logchan chan string) {
        var logFile *os.File
        
        //get current datetime and creates filename based on current time
        now := time.Now()
        filename := ("sinkhole-" + now.Format("2006-01-02-15-04-05") + ".log")
        
        //create file
        logFile, err = os.OpenFile(filename, os.O_CREATE|os.O_RDWR, 0666)
        if err != nil {
                log.Fatal(err)
        }
        
        defer logfile.Close()

        for l := range Logchan {
                n, err := io.WriteString(logFile, l + "\n")
                if err != nil {
                        fmt.Println(n, err)
                }
        }
}
