package main

/*
map[Location:[yeelight://10.10.200.205:55443] Server:[POSIX UPnP/1.0 YGLC/1] Model:[mono] Fw_ver:[40]
Support:[get_prop set_default set_power toggle set_bright start_cf stop_cf set_scene cron_add cron_get cron_del set_adjust set_name]
Power:[off] Ct:[4000] Rgb:[0] Cache-Control:[max-age=3600] Id:[0x0000000003360248]
Sat:[0] Name:[White] Date:[] Ext:[] Bright:[100] Color_mode:[2] Hue:[0]]
*/

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
)

const (
	// OFF Light off
	OFF = iota
	// ON Light on
	ON = iota
	// OFFLINE Light unreachable
	OFFLINE = iota
)

// Yeelight is a light.
type Yeelight struct {
	Address   string
	Name      string
	ID        string
	Model     string
	FW        int
	Power     int
	Bright    int
	Sat       int
	CT        int
	RGB       int
	Hue       int
	ColorMode int
	Support   map[string]bool
	ReqCount  int
	Conn      *net.TCPConn
}

// Command JSON commands sent to lights
type Command struct {
	ID     int      `json:"id"`
	Method string   `json:"method"`
	Params []string `json:"params"`
}

var (
	errWithoutYeelightPrefix = errors.New("Yeelight prefix not found")
	errResolveTCP            = errors.New("Cannot resolve TCP address")
	errConnectLight          = errors.New("Cannot connect to light")
	errCommandNotSupported   = errors.New("Command not supported")
	errNotConnected          = errors.New("Light not connected")
)

// parseYeelight returns a Yeelight based on the
// HTTP headers of its SSDP response represented by header
// it returns an error if something goes wrong during parsing
func parseYeelight(header http.Header) (*Yeelight, error) {
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
		Address:   addr[11:],
		Name:      header.Get("Name"),
		ID:        header.Get("Id"),
		Model:     header.Get("Model"),
		FW:        fw,
		Power:     power,
		Bright:    bright,
		Sat:       sat,
		CT:        ct,
		RGB:       rgb,
		Hue:       hue,
		ColorMode: color,
		Support:   support,
		ReqCount:  0,
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

var endOfCommand = []byte{'\r', '\n'}

// SendCommand sends "comm" command to a light with variable parameters
func (l *Yeelight) SendCommand(comm string, params ...string) error {
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
	fmt.Println(string(jCmd))

	jCmd = bytes.Join([][]byte{jCmd, endOfCommand}, nil)
	_, err = l.Conn.Write(jCmd)
	if err != nil {
		return err
	}
	l.ReqCount++
	return nil
}

// Toogle toogle light's power on/off
func (l *Yeelight) Toggle() error {
	return l.SendCommand("toggle", "")
}
