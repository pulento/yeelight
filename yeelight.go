package yeelight

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	ssdp "github.com/pulento/go-ssdp"
)

var (
	mcastAddress  = "239.255.255.250:1982"
	searchType    = "wifi_bulb"
	connTimeout   = time.Duration(3) * time.Second
	refreshPeriod = time.Duration(30) * time.Second
)

// Search search for lights from some time using SSDP and
// returns a map of lights found indexed by its ID
func Search(time int, localAddr string) (map[string]*Light, error) {
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
	lightsMap := make(map[string]*Light)
	for _, srv := range list {
		light, err := Parse(srv.Header())
		if err != nil {
			log.Printf("Invalid response from %s: %s", srv.Location, err)
			return nil, err
		}
		// Lights respond multiple times to a search
		if lightsMap[light.ID] == nil {
			lightsMap[light.ID] = light
		}
	}
	//log.Println(lightsMap)
	return lightsMap, nil
}

// SSDPMonitor starts listening light's SSDP traffic
// lightmap is a map of *Light so it can update it with
// lights founded, lightfound is called for each light notification
func SSDPMonitor(lightmap map[string]*Light, lightfound func(light *Light)) error {
	err := ssdp.SetMulticastRecvAddrIPv4(mcastAddress)
	if err != nil {
		return err
	}
	mon := &ssdp.Monitor{
		Alive: func(m *ssdp.AliveMessage) {
			lightAlive(lightmap, m, lightfound)
		},
	}
	if err := mon.Start(); err != nil {
		return err
	}
	return nil
}

func lightAlive(lm map[string]*Light, m *ssdp.AliveMessage, lightfound func(light *Light)) {
	light, err := Parse(m.Header())
	if err != nil {
		log.Printf("Invalid SSDP notification from %s: %s", m.Location, err)
		return
	}
	//log.Printf("SSDP notification Light %s named %s from %s: %v",
	//	light.ID, light.Name, m.From.String(), *light)
	// Add it to the map if is a new light
	if lm[light.ID] == nil {
		lm[light.ID] = light
	} else {
		// Updates existing light
		Copy(lm[light.ID], light)
	}
	lm[light.ID].LastSeen = time.Now().Unix()
	lm[light.ID].refresh = time.After(refreshPeriod)
	// Call the callback
	lightfound(lm[light.ID])
}

// Copy copies just light's values
// Omitting internal entities like channels, sockets, etc
func Copy(dst *Light, src *Light) {
	dst.ID = src.ID
	dst.Name = src.Name
	dst.Address = src.Address
	dst.Model = src.Model
	dst.CacheControl = src.CacheControl
	dst.FW = src.FW
	dst.Power = src.Power
	dst.Bright = src.Bright
	dst.Sat = src.Sat
	dst.CT = src.CT
	dst.RGB = src.RGB
	dst.Hue = src.Hue
	dst.ColorMode = src.ColorMode
	dst.Support = src.Support
}

// Parse returns a Yeelight based on the
// HTTP headers of its SSDP response represented by header
// it returns an error if something goes wrong during parsing
func Parse(header http.Header) (*Light, error) {
	addr := header.Get("Location")
	if !strings.HasPrefix(addr, "yeelight://") {
		return nil, errWithoutYeelightPrefix
	}

	fw, err := strconv.Atoi(header.Get("FW_Ver"))
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
		Calls:        make(map[int32]*Command),
		ResC:         make(chan *Result, 1),
	}
	return light, nil
}

// Connect connects to a light
func (l *Light) Connect() error {
	d := net.Dialer{Timeout: connTimeout}
	cn, err := d.Dial("tcp", l.Address)
	if err != nil {
		return err
	}
	l.Conn = cn.(*net.TCPConn)
	l.Reader = bufio.NewReader(l.Conn)
	l.LastSeen = time.Now().Unix()
	l.refresh = time.After(refreshPeriod)
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

// This is to send received data and error on the
// same channel to the Listener
type message struct {
	mess string
	err  error
}

// Receives data from light should be span on a goroutine
func (l *Light) receiver(d chan<- *message, done <-chan bool) {
	for {
		select {
		case <-done:
			return
		default:
			data, err := l.Message()
			if err != nil {
				log.Println("receiver: Error receiving message:", err)
			}
			d <- &message{data, err}
		}
	}
}

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
		//make sure connection is closed when method returns
		defer l.Close()

		mes := make(chan *message)
		rdone := make(chan bool)
		go l.receiver(mes, rdone)
		defer func() {
			rdone <- true
		}()

		for {
			var resnot *ResultNotification

			select {
			case <-done:
				goto exit
			case <-l.refresh:
				log.Println("Periodic Refresh:", l.ID)
				l.refresh = time.After(refreshPeriod)
			case d := <-mes:
				if d.err == nil {
					err := json.Unmarshal([]byte(d.mess), &resnot)
					if err != nil {
						log.Println("Error parsing message:", err)
					}
					if resnot.Notification != nil {
						resnot.Notification.DevID = l.ID
						l.processNotification(resnot.Notification)
					}
					if resnot.Result != nil {
						resnot.Result.DevID = l.ID
						l.processResult(resnot.Result)
					}
					notifCh <- resnot
				} else {
					if err == io.EOF {
						log.Printf("Connection closed for %s [%s] to %s. Trying reconnect", l.ID, l.Name, l.Address)
						err = l.Connect()
						if err != nil {
							log.Println("Error reconnecting to", l.Address)
							goto exit
						}
					}
					log.Println("Error receiving message:", err)
				}
			}
		}
	exit:
		return
	}(l.Conn)

	return done, nil
}

func (l *Light) processNotification(n *Notification) error {
	mapNotificationS := map[string]*string{
		"name":          &l.Name,
		"id":            &l.ID,
		"model":         &l.Model,
		"cache-control": &l.CacheControl,
	}
	mapNotificationI := map[string]*int{
		"fw_ver":     &l.FW,
		"bright":     &l.Bright,
		"color_mode": &l.ColorMode,
		"ct":         &l.CT,
		"rgb":        &l.RGB,
		"hue":        &l.Hue,
		"sat":        &l.Sat,
	}

	if n.Method == "props" {
		//log.Println(n.Params)
		for k, v := range n.Params {
			if k == "power" {
				if v == "on" {
					l.Power = 1
				} else {
					l.Power = 0
				}
			}
		}
		// FIXME: JSON dedicated struct for params would be better ?
		for k, v := range mapNotificationI {
			if n.Params[k] != nil {
				*v = int(n.Params[k].(float64))
			}
		}
		for k, v := range mapNotificationS {
			if n.Params[k] != nil {
				str := (n.Params[k]).(string)
				if str != "" {
					*v = str
				}
			}
		}
	}
	return nil
}

func (l *Light) processResult(r *Result) error {
	if l.Calls[int32(r.ID)] != nil {
		delete(l.Calls, int32(r.ID))
		l.ResC <- r
	} else {
		log.Println("Received reply to unknown request:", r.ID)
	}
	return nil
}

// SendCommand sends "comm" command to a light with "params" parameters
// returning the request ID for tracking results
func (l *Light) SendCommand(comm string, params ...interface{}) (int32, error) {
	if !l.Support[comm] {
		return -1, errCommandNotSupported
	}
	if l.Conn == nil {
		return -1, errNotConnected
	}
	cmd := &Command{
		ID:     atomic.LoadInt32(&l.ReqCount),
		Method: comm,
		Params: params,
	}
	jCmd, err := json.Marshal(cmd)
	log.Printf("Sending command %s to %s at %s", string(jCmd), l.Name, l.Address)

	jCmd = bytes.Join([][]byte{jCmd, endOfCommand}, nil)
	_, err = l.Conn.Write(jCmd)
	if err != nil {
		return -1, err
	}
	l.Calls[cmd.ID] = cmd
	return (atomic.AddInt32(&l.ReqCount, 1) - 1), nil
}

// WaitResult waits timeout seconds for a result on a request with res ID
func (l *Light) WaitResult(res int32, timeout int) *Result {
	select {
	case r := <-l.ResC:
		if int32(r.ID) == res {
			return r
		}
	case <-time.After(time.Duration(timeout) * time.Second):
		return nil
	}
	return nil
}

// Message gets light messages
func (l *Light) Message() (string, error) {
	if l.Conn == nil {
		return "", errNotConnected
	}
	resp, err := l.Reader.ReadString('\n')

	if err != nil {
		return "", err
	}
	//log.Printf("Message: Message from %s at %s: %s", l.Name, l.Address, resp)
	l.LastSeen = time.Now().Unix()
	l.refresh = time.After(refreshPeriod)
	return resp, nil
}

// Toggle toogle light's power on/off
func (l *Light) Toggle() (int32, error) {
	return l.SendCommand("toggle", "")
}

// SetPower set light's power with effect of duration
func (l *Light) SetPower(power bool, effect int, duration int) (int32, error) {
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
func (l *Light) SetBrightness(brightness int, duration int) (int32, error) {
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
func (l *Light) SetTemperature(temp int, duration int) (int32, error) {
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
func (l *Light) SetRGB(rgb uint32, duration int) (int32, error) {
	var str string

	if rgb > 0xffffff {
		return 0, errInvalidParam
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
func (l *Light) SetHSV(hsv uint16, sat uint8, duration int) (int32, error) {
	var str string

	if sat > 100 || hsv > 359 {
		return 0, errInvalidParam
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
func (l *Light) SetName(name string, duration int) (int32, error) {
	return l.SendCommand("set_name", name)
}

// GetProp gets light properties
func (l *Light) GetProp(props ...interface{}) (int32, error) {
	return l.SendCommand("get_prop", props...)
}
