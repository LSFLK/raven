package models

import "net"

type ClientState struct {
	Authenticated  bool
	SelectedFolder string
	Conn           net.Conn
	Username       string
}
