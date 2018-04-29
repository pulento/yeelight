package yeelight

import (
	"errors"
	"net"
)

const (
	// OFF Light off
	OFF = iota
	// ON Light on
	ON = iota
	// OFFLINE Light unreachable
	OFFLINE = iota
)

// Light is the light :)
type Light struct {
	Address      string
	Name         string
	ID           string
	Model        string
	CacheControl string
	FW           int
	Power        int
	Bright       int
	Sat          int
	CT           int
	RGB          int
	Hue          int
	ColorMode    int
	Support      map[string]bool
	ReqCount     int
	Conn         *net.TCPConn
}

// Command JSON commands sent to lights
type Command struct {
	ID     int           `json:"id"`
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
	Method string            `json:"method"`
	Params map[string]string `json:"params"`
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
)
