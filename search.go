package main

import (
	"flag"
	"log"
	"os"
	"time"

	"github.com/pulento/go-ssdp"
)

var (
	mcastAddress = "239.255.255.250:1982"
	searchType   = "wifi_bulb"
)

func main() {
	w := flag.Int("w", 1, "\twait time")
	l := flag.String("l", "", "\tlocal address to listen")
	v := flag.Bool("v", false, "\tverbose mode")
	h := flag.Bool("h", false, "\tshow help")
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
	lightsMap := make(map[string]Yeelight)
	var lights []*Yeelight
	for _, srv := range list {
		light, err := parseYeelight(srv.Header())
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

	for i, l := range lights {
		err := l.Connect()
		if err != nil {
			log.Printf("Error connecting to %s: %s", l.Address, err)
		} else {
			log.Printf("Light #%d named %s connected to %s", i, l.Name, l.Address)
		}
	}
	for _, l := range lights {
		prop := "power"
		err := l.GetProp(prop, "bright")
		if err != nil {
			log.Printf("Error getting property %s on %s: %s", prop, l.Address, err)
		}
		err = l.Response()
		if err != nil {
			log.Printf("Error getting response from %s: %s", l.Address, err)
		}
	}

	for _, l := range lights {
		err := l.Toggle()
		if err != nil {
			log.Printf("Error toggling %s: %s", l.Address, err)
		}
		err = l.Response()
		if err != nil {
			log.Printf("Error getting response from %s: %s", l.Address, err)
		}
		time.Sleep(1 * time.Second)
		err = l.Toggle()
		if err != nil {
			log.Printf("Error toggling %s: %s", l.Address, err)
		}
		err = l.Response()
		if err != nil {
			log.Printf("Error getting response from %s: %s", l.Address, err)
		}
	}
	log.Println("Lights:", lights)
}
