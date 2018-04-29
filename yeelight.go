package yeelight

/*
map[Location:[yeelight://10.10.200.205:55443] Server:[POSIX UPnP/1.0 YGLC/1] Model:[mono] Fw_ver:[40]
Support:[get_prop set_default set_power toggle set_bright start_cf stop_cf set_scene cron_add cron_get cron_del set_adjust set_name]
Power:[off] Ct:[4000] Rgb:[0] Cache-Control:[max-age=3600] Id:[0x0000000003360248]
Sat:[0] Name:[White] Date:[] Ext:[] Bright:[100] Color_mode:[2] Hue:[0]]
*/

import (
	"bufio"
	"bytes"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
)

// Parse returns a Yeelight based on the
// HTTP headers of its SSDP response represented by header
// it returns an error if something goes wrong during parsing
func Parse(header http.Header) (*Yeelight, error) {
	addr := header.Get("Location")
	if !strings.HasPrefix(addr, "yeelight://") {
		return nil, errWithoutYeelightPrefix
	}

	fw, err := strconv.Atoi(header.Get("FW"))
	bright, err := strconv.Atoi(header.Get("Bright"))
	sat, err := strconv.Atoi(header.Get("Sat"))
	ct, err := strconv.Atoi(header.Get("Ct"))
	rgb, err := strconv.Atoi(header.Get("Rgb"))
	hue, err := strconv.Atoi(header.Get("Hue"))
	color, err := strconv.Atoi(header.Get("Color_mode"))
	power := OFFLINE

	p := header.Get("Power")
	if p == "on" {
		power = ON
	} else if p == "off" {
		power = OFF
	}

	if err != nil {
		return nil, err
	}

	// Create a map of supported commands
	slist := strings.Split(header.Get("Support"), " ")
	support := make(map[string]bool, len(slist))
	for _, v := range slist {
		support[v] = true
	}

	light := &Yeelight{
		Address:      addr[11:],
		Name:         header.Get("Name"),
		ID:           header.Get("Id"),
		Model:        header.Get("Model"),
		CacheControl: header.Get("Cache-Control"),
		FW:           fw,
		Power:        power,
		Bright:       bright,
		Sat:          sat,
		CT:           ct,
		RGB:          rgb,
		Hue:          hue,
		ColorMode:    color,
		Support:      support,
		ReqCount:     0,
	}
	return light, nil
}

// Connect connects to a light
func (l *Yeelight) Connect() error {
	tcpAddr, err := net.ResolveTCPAddr("tcp", l.Address)
	if err != nil {
		return err
	}

	cn, err := net.DialTCP("tcp", nil, tcpAddr)
	l.Conn = cn
	if err != nil {
		return err
	}
	return nil
}

// Close closes the connection to light
func (l *Yeelight) Close() error {
	err := l.Conn.Close()
	if err != nil {
		return err
	}
	return nil
}

var endOfCommand = []byte{'\r', '\n'}

// Listen connects to light and listens for events
// which are sent to notifCh
func (l *Yeelight) Listen(notifCh chan<- *ResultNotification) (chan<- bool, error) {
	done := make(chan bool)

	err := l.Connect()
	if err != nil {
		return nil, err
	}
	log.Printf("Listening Connection established for %s on %s", l.Name, l.Address)
	go func(c net.Conn) {
		var notif Notification
		var result Result
		var resnot *ResultNotification
		//make sure connection is closed when method returns
		defer l.Close()

		for {
			data, err := l.Message()
			if err == nil {
				log.Printf("Sending to Channel: %s from %s at %s", strings.TrimSuffix(data, "\r\n"), l.Name, l.Address)
				json.Unmarshal([]byte(data), &notif)
				json.Unmarshal([]byte(data), &result)
				if notif.Method != "" {
					resnot = &ResultNotification{nil, &notif}
				} else {
					resnot = &ResultNotification{&result, nil}
				}
			}
			select {
			case <-done:
				return
			case notifCh <- resnot:
				notifCh <- resnot
				//log.Println("Data sent to channel")
			}
		}
	}(l.Conn)

	return done, nil
}

// SendCommand sends "comm" command to a light with variable parameters
func (l *Yeelight) SendCommand(comm string, params ...interface{}) error {
	if !l.Support[comm] {
		return errCommandNotSupported
	}
	if l.Conn == nil {
		return errNotConnected
	}
	cmd := &Command{
		ID:     l.ReqCount,
		Method: comm,
		Params: params,
	}
	jCmd, err := json.Marshal(cmd)
	log.Printf("Sending command %s to %s at %s", string(jCmd), l.Name, l.Address)

	jCmd = bytes.Join([][]byte{jCmd, endOfCommand}, nil)
	_, err = l.Conn.Write(jCmd)
	if err != nil {
		return err
	}
	l.ReqCount++
	return nil
}

// Message gets light messages
func (l *Yeelight) Message() (string, error) {
	if l.Conn == nil {
		return "", errNotConnected
	}
	connReader := bufio.NewReader(l.Conn)
	resp, err := connReader.ReadString('\n')

	if err != nil {
		return "", err
	}
	//log.Printf("Message from %s at %s: %s", l.Name, l.Address, resp)
	return resp, nil
}

// Toggle toogle light's power on/off
func (l *Yeelight) Toggle() error {
	return l.SendCommand("toggle", "")
}

const (
	// SUDDEN effect
	SUDDEN = iota
	// SMOOTH effect
	SMOOTH = iota
)

// SetPower set light's power with effect of duration
func (l *Yeelight) SetPower(power bool, effect int, duration int) error {
	var str, p string
	if power {
		p = "on"
	} else {
		p = "off"
	}
	if effect == SUDDEN {
		str = "sudden"
	} else if effect == SMOOTH {
		str = "smooth"
	}
	return l.SendCommand("set_bright", p, str, duration)
}

// SetBrightness set light's brightness with effect of duration
func (l *Yeelight) SetBrightness(brightness int, effect int, duration int) error {
	var str string

	if effect == SUDDEN {
		str = "sudden"
	} else if effect == SMOOTH {
		str = "smooth"
	}
	return l.SendCommand("set_bright", brightness, str, duration)
}

// GetProp gets light properties
func (l *Yeelight) GetProp(props ...interface{}) error {
	return l.SendCommand("get_prop", props...)
}
