package main

import (
        "log"
        "log/syslog"
        "fmt"
        "os"
        "io"
        "time"
	"sync"
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
	var err error
	var newfilemutex = &sync.Mutex{}

        //ticker and file rotation goroutine
        ticker := time.NewTicker(time.Minute * 10)
        go func() {
        	//get current datetime and creates filename based on current time
		newfilemutex.Lock()
        	now := time.Now()
        	filename := ("sinkhole-" + now.Format("2006-01-02-15-04-05") + ".log")

        	//create inital file
        	logFile, err = os.OpenFile(filename, os.O_CREATE|os.O_RDWR, 0666)
		newfilemutex.Unlock()
        	if err != nil {
                	log.Fatal(err)
       		}

                for range ticker.C {
			newfilemutex.Lock()
                        logFile.Close()
			//get current datetime and creates filename based on current time
        		now := time.Now()
                        filename := ("sinkhole-" + now.Format("2006-01-02-15-04-05") + ".log")
                        logFile, err = os.OpenFile(filename, os.O_CREATE|os.O_RDWR, 0666)
                        if err != nil {
                                log.Fatal(err)
                        }
			newfilemutex.Unlock()
                }
        }()


        defer logFile.Close()

        for l := range Logchan {
		newfilemutex.Lock()
		n, err := io.WriteString(logFile, l + "\n")
		if err != nil {
			fmt.Println(n, err)
		}
		newfilemutex.Unlock()
	}

}
