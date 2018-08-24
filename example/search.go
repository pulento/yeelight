package main

import (
	"flag"
	"log"
	"sync"
	"time"

	"github.com/pulento/yeelight"
)

func main() {
	var wg sync.WaitGroup
	w := flag.Int("w", 1, "\tSSDP wait time")
	l := flag.String("l", "", "\tlocal address to listen")
	h := flag.Bool("h", false, "\tshow help")
	t := flag.Int("t", 3, "\tListeners wait time")
	flag.Parse()
	if *h {
		flag.Usage()
		return
	}

	lights := make(map[string]*yeelight.Light)
	resnot := make(chan *yeelight.ResultNotification)
	done := make(chan bool)

	err := yeelight.Search(*w, *l, lights, func(l *yeelight.Light) {
		_, lerr := l.Listen(resnot)
		if lerr != nil {
			log.Printf("Error connecting to %s: %s", l.Address, lerr)
		} else {
			log.Printf("Light %s named %s connected to %s", l.ID, l.Name, l.Address)
		}
	})
	if err != nil {
		log.Fatal("Error searching lights cannot continue:", err)
	}

	log.Printf("Waiting for lights events for %d [sec]", *t)

	wg.Add(1)
	go func(c <-chan *yeelight.ResultNotification, done <-chan bool) {
		defer wg.Done()
		log.Println("Channel receiver started")
		for {
			select {
			case <-c:
				{
					data := <-c
					if data.Notification != nil {
						log.Println("Notification from Channel", *data.Notification)
					} else {
						log.Println("Result from Channel", *data.Result)
					}
				}
			case <-done:
				return
			}
		}
	}(resnot, done)

	for _, l := range lights {
		prop := "power"
		_, err := l.GetProp(prop, "bright")
		if err != nil {
			log.Printf("Error getting property %s on %s: %s", prop, l.Address, err)
		}
	}

	time.Sleep(time.Duration(*t) * time.Second)
	done <- true
	wg.Wait()
	log.Println("Lights:", lights)

}
