package yeelight

import (
	"bufio"
	"errors"
	"net"
	"time"
)

// Light's power
const (
	// OFF Light off
	OFF = iota
	// ON Light on
	ON = iota
	// OFFLINE Light unreachable
	UNKNOWN = iota
)

// Light's connectivity
const (
	OFFLINE  = 0
	SSDP     = 1
	UPDATING = 2
	ONLINE   = 3
)

// Light is the light :)
type Light struct {
	Address      string          `json:"address"`
	Name         string          `json:"name"`
	ID           string          `json:"id"`
	Model        string          `json:"model"`
	CacheControl string          `json:"cache-control"`
	FW           int             `json:"fw"`
	Power        string          `json:"power"`
	Bright       int             `json:"bright"`
	Sat          int             `json:"sat"`
	CT           int             `json:"ct"`
	RGB          int             `json:"rgb"`
	Hue          int             `json:"hue"`
	ColorMode    int             `json:"color-mode"`
	Support      map[string]bool `json:"support"`
	ReqCount     int32           `json:"reqcount"`
	LastSeen     int64           `json:"lastseen"`
	Status       int32           `json:"status"`
	refresh      <-chan time.Time
	Conn         *net.TCPConn       `json:"-"`
	Calls        map[int32]*Command `json:"-"`
	ResC         chan *Result       `json:"-"`
	Reader       *bufio.Reader      `json:"-"`
}

// Command JSON commands sent to lights
type Command struct {
	ID     int32         `json:"id"`
	Method string        `json:"method"`
	Params []interface{} `json:"params"`
}

// Result represent results to commands from lights
type Result struct {
	DevID  string
	ID     int           `json:"id"`
	Result []interface{} `json:"result,omitempty"`
	Error  *Error        `json:"error,omitempty"`
}

// Notification represents notification response
type Notification struct {
	DevID  string
	Method string                 `json:"method"`
	Params map[string]interface{} `json:"params"`
}

// Error codes from lights
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ResultNotification is the generic response
type ResultNotification struct {
	*Result
	*Notification
}

var (
	errWithoutYeelightPrefix = errors.New("Yeelight prefix not found")
	errResolveTCP            = errors.New("Cannot resolve TCP address")
	errConnectLight          = errors.New("Cannot connect to light")
	errCommandNotSupported   = errors.New("Command not supported")
	errNotConnected          = errors.New("Light not connected")
	errInvalidParam          = errors.New("Invalid parameter value")
)
