package main

import (
        "gopkg.in/natefinch/lumberjack.v2"
        "log"
        "log/syslog"
        "fmt"
        "os"
        "io"
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

func RotateLogFile(file *os.File) {
                
}

func writeLogger(Logchan chan string) {
        var logFile *os.File

        //check to see if log file exsits already
        if _, err := os.Stat("/path/to/whatever"); os.IsNotExist(err) {
                // if file does not exist then create the file
                logFile, err = os.OpenFile("test.log", os.O_CREATE|os.O_RDWR, 0666)
                if err != nil {
                        log.Fatal(err)
                }
        } else {
                // if the file does exist truncate it
                logFile, err = os.OpenFile("test.log", os.O_TRUNC|os.O_RDWR, 0666)
                if err != nil {
                        log.Fatal(err)
                }
        }

        for l := range Logchan {
                n, err := io.WriteString(logFile, l + "\n")
                if err != nil {
                        fmt.Println(n, err)
                }
        }
}
