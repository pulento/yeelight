package main

import (
	"flag"
	"log"
	"os"
	"sync"
	"time"

	"bitbucket.org/pulento/yeelight"

	"github.com/pulento/go-ssdp"
)

var (
	mcastAddress = "239.255.255.250:1982"
	searchType   = "wifi_bulb"
)

func main() {
	var wg sync.WaitGroup
	w := flag.Int("w", 1, "\tSSDP wait time")
	l := flag.String("l", "", "\tlocal address to listen")
	v := flag.Bool("v", false, "\tverbose mode")
	h := flag.Bool("h", false, "\tshow help")
	t := flag.Int("t", 3, "\tListeners wait time")
	flag.Parse()
	if *h {
		flag.Usage()
		return
	}
	if *v {
		ssdp.Logger = log.New(os.Stderr, "[SSDP] ", log.LstdFlags)
	}

	err := ssdp.SetMulticastSendAddrIPv4(mcastAddress)
	if err != nil {
		log.Fatal(err)
	}

	list, err := ssdp.Search(searchType, *w, *l)
	if err != nil {
		log.Fatal(err)
	}

	// Create a map based on light's ID
	lightsMap := make(map[string]yeelight.Light)
	var lights []*yeelight.Light
	for _, srv := range list {
		light, err := yeelight.Parse(srv.Header())
		if err != nil {
			log.Printf("Invalid response from %s: %s", srv.Location, err)
			os.Exit(1)
		}
		// Lights respond multiple times to a search
		// Create a map of unique lights ID
		if lightsMap[light.ID].ID == "" {
			lightsMap[light.ID] = *light
			lights = append(lights, light)
		}
	}

	resnot := make(chan *yeelight.ResultNotification)
	done := make(chan bool)
	log.Printf("Waiting for lights events for %d [sec]", *t)
	for i, l := range lights {
		_, err = l.Listen(resnot)
		if err != nil {
			log.Printf("Error connecting to %s: %s", l.Address, err)
		} else {
			log.Printf("Light #%d named %s connected to %s", i, l.Name, l.Address)
		}
	}

	wg.Add(1)
	go func(c <-chan *yeelight.ResultNotification, done <-chan bool) {
		defer wg.Done()
		log.Println("Channel receiver started")
		for {
			select {
			case <-c:
				data := <-c
				if data.Notification != nil {
					log.Println("Notification from Channel", *data.Notification)
				} else {
					log.Println("Result from Channel", *data.Result)
				}
			case <-done:
				return
			}
		}
	}(resnot, done)

	for _, l := range lights {
		prop := "power"
		err := l.GetProp(prop, "bright")
		if err != nil {
			log.Printf("Error getting property %s on %s: %s", prop, l.Address, err)
		}
		/*_, err = l.Message()
		if err != nil {
			log.Printf("Error getting response from %s: %s", l.Address, err)
		}*/
	}

	time.Sleep(time.Duration(*t) * time.Second)
	done <- true
	wg.Wait()
	log.Println("Lights:", lights)

}
