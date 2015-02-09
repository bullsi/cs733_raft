package raft
import (
		"time"
		"net"
		)

type Lsn uint64 //Log sequence number, unique for all time.

type Value struct {
	Text			string
	ExpiryTime		time.Time
	IsExpTimeInf	bool
	NumBytes		int
	Version			int64
}

type String_Conn struct {
	Text string
	Conn net.Conn
}
type LogEntry interface {
	Lsn() Lsn
	Data() []byte
	Committed() bool
}
type SharedLog interface {
	Init()
	Append(data []byte) (LogEntry, error)
	Commit(sequenceNumber Lsn, conn net.Conn)
}

var KVStore = make(map[string]Value)

var Input_ch = make(chan String_Conn, 10000)
var Append_ch = make(chan LogEntry, 10000)
var Commit_ch = make(chan Lsn, 10000)
var Output_ch = make(chan String_Conn, 10000)



