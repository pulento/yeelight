package yeelight

import (
	"bufio"
	"bytes"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	ssdp "github.com/pulento/go-ssdp"
)

var (
	mcastAddress = "239.255.255.250:1982"
	searchType   = "wifi_bulb"
	connTimeout  = 3 * time.Second
)

// Search search for lights from some time using SSDP and
// returns a list of lights found
func Search(time int, localAddr string) ([]*Light, error) {
	//ssdp.Logger = log.New(os.Stderr, "[SSDP] ", log.LstdFlags)
	err := ssdp.SetMulticastSendAddrIPv4(mcastAddress)
	if err != nil {
		return nil, err
	}

	list, err := ssdp.Search(searchType, time, localAddr)
	if err != nil {
		return nil, err
	}

	// Create a map based on light's ID
	lightsMap := make(map[string]Light)
	var lights []*Light
	for _, srv := range list {
		light, err := Parse(srv.Header())
		if err != nil {
			log.Printf("Invalid response from %s: %s", srv.Location, err)
			return nil, err
		}
		// Lights respond multiple times to a search
		// Create a map of unique lights ID
		if lightsMap[light.ID].ID == "" {
			lightsMap[light.ID] = *light
			lights = append(lights, light)
		}
	}
	return lights, nil
}

// Parse returns a Yeelight based on the
// HTTP headers of its SSDP response represented by header
// it returns an error if something goes wrong during parsing
func Parse(header http.Header) (*Light, error) {
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

	light := &Light{
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
func (l *Light) Connect() error {
	d := net.Dialer{Timeout: connTimeout}
	cn, err := d.Dial("tcp", l.Address)

	l.Conn = cn.(*net.TCPConn)
	if err != nil {
		return err
	}
	return nil
}

// Close closes the connection to light
func (l *Light) Close() error {
	err := l.Conn.Close()
	if err != nil {
		return err
	}
	return nil
}

var endOfCommand = []byte{'\r', '\n'}

// Listen connects to light and listens for events
// which are sent to notifCh
func (l *Light) Listen(notifCh chan<- *ResultNotification) (chan<- bool, error) {
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
					notif.DevID = l.ID
					resnot = &ResultNotification{nil, &notif}
				} else {
					result.DevID = l.ID
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
func (l *Light) SendCommand(comm string, params ...interface{}) error {
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
func (l *Light) Message() (string, error) {
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
func (l *Light) Toggle() error {
	return l.SendCommand("toggle", "")
}

const (
	// SUDDEN effect
	SUDDEN = iota
	// SMOOTH effect
	SMOOTH = iota
)

// SetPower set light's power with effect of duration
func (l *Light) SetPower(power bool, effect int, duration int) error {
	var str, p string
	if power {
		p = "on"
	} else {
		p = "off"
	}
	if duration > 0 {
		str = "smooth"
	} else {
		str = "sudden"
		duration = 0
	}
	return l.SendCommand("set_bright", p, str, duration)
}

// SetBrightness set light's brightness with effect of duration
func (l *Light) SetBrightness(brightness int, duration int) error {
	var str string

	if duration > 0 {
		str = "smooth"
	} else {
		str = "sudden"
		duration = 0
	}
	return l.SendCommand("set_bright", brightness, str, duration)
}

// SetTemperature set light's color temperature with effect of duration
func (l *Light) SetTemperature(temp int, duration int) error {
	var str string

	if duration > 0 {
		str = "smooth"
	} else {
		str = "sudden"
		duration = 0
	}
	return l.SendCommand("set_ct_abx", temp, str, duration)
}

// SetRGB set light's color in RGB format with effect of duration
func (l *Light) SetRGB(rgb uint32, duration int) error {
	var str string

	if rgb > 0xffffff {
		return errInvalidParam
	}
	if duration > 0 {
		str = "smooth"
	} else {
		str = "sudden"
		duration = 0
	}
	return l.SendCommand("set_rgb", rgb, str, duration)
}

// SetHSV set light's color in HSV format with effect of duration
func (l *Light) SetHSV(hsv uint16, sat uint8, duration int) error {
	var str string

	if sat > 100 || hsv > 359 {
		return errInvalidParam
	}
	if duration > 0 {
		str = "smooth"
	} else {
		str = "sudden"
		duration = 0
	}
	return l.SendCommand("set_hsv", hsv, sat, str, duration)
}

// SetName set light's name
func (l *Light) SetName(name string, duration int) error {
	return l.SendCommand("set_name", name)
}

// GetProp gets light properties
func (l *Light) GetProp(props ...interface{}) error {
	return l.SendCommand("get_prop", props...)
}
