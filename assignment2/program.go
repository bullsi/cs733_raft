package main

import (
	"raft"
	"fmt"
	"log"
	"time"
	"encoding/gob"
	"strings"
	"net"
	"net/rpc"
	"os"
	"strconv"
)

var N int = 3			// Number of servers
// *************************************************************

/////////////////////////////////////////////////
// Reads send-requests from the output channel and processes it
func DataWriter() {
	for {
		replych := <-raft.Output_ch
		text := replych.Text
		conn := replych.Conn			// nil for followers as they don't have to send the reply back
		if text != "" && conn != nil {
			conn.Write([]byte(text))
		}
	}
}

//
func AppendCaller() {
	for {
		logentry := <-raft.Append_ch
		// Append it to everyone's log
		for i:=0; i<len(r.clusterConfig.Servers); i++ {
			if i == r.id { continue }
			args := &AppendRPCArgs{logentry}
			var reply string
			err := r.clusterConfig.Servers[i].Client.Call("RPC.AppendRPC", args, &reply)
			if err != nil {
				log.Println("[Server] AppendRPC Error:", err)
			}
			log.Print("[Server] ", reply, " from ", r.clusterConfig.Servers[i].LogPort)
		}
	}
}

func CommitCaller() {
	for {
		lsn := <-raft.Commit_ch
		// Commit it to everyone's KV Store
		for i:=0; i<len(r.clusterConfig.Servers); i++ {
			if i == r.id { continue }
			args := &CommitRPCArgs{lsn}
			var reply string
			err := r.clusterConfig.Servers[i].Client.Call("RPC.CommitRPC", args, &reply)
			if err != nil {
				log.Println("[Server] CommitRPC Error:", err)
			}
//			log.Print("[Server] ", reply, " from ", r.clusterConfig.Servers[i].LogPort)
		}
	}
}

// Retrieves the next command out of the input buffer
func GetCommand(input string) (string, string) {
	inputs := strings.Split(input, "\r\n")
	n1 := len(inputs)
	n := len(inputs[0])
//		abc := input[0:3]
//		log.Printf("**%s--%s--%s--%s-", input, inputs[0], (inputs[0])[1:3], abc)
		
	com, rem := "", ""
	if n >= 3 && (inputs[0][0:3] == "set" || inputs[0][0:3] == "cas") {
		// start of a 2 line command
		if n1 < 3 {						// includes "\r\n"
			return "", input			// if the command is not complete, wait for the rest of the command
		}
		var in = strings.Index(input, "\r\n") + 2
		in += strings.Index(input[in:], "\r\n") + 2
		com = input[:in]
		rem = input[in:]
	} else if (n >= 3 && inputs[0][0:3] == "get") ||
		(n >= 4 && inputs[0][0:4] == "getm") ||
		(n >= 6 && inputs[0][0:6] == "delete") {
		// start of a 1 line command
		if n1 < 2 {						// includes "\r\n"
			return "", input			// if the command is not complete, wait for the rest of the command
		}
		var in = strings.Index(input, "\r\n") + 2
		com = input[:in]
		rem = input[in:]
		
	} else {
		return "", input
	}
	return com, rem
}

func Trim(input []byte) []byte {
	i := 0
	for ; input[i] != 0; i++ {}
	return input[0:i]
}

// *************************************************************

var r *Raft			// Contains the server details
// ----------------------
type LogEntry_ struct {
	Term int
	SequenceNumber raft.Lsn
	Command []byte
	IsCommitted bool
}
func (l LogEntry_) Lsn() raft.Lsn {
	return l.SequenceNumber
}
func (l LogEntry_) Data() []byte {
	return l.Command
}
func (l LogEntry_) Committed() bool {
	return l.IsCommitted
}

// ----------------------------------------------
type SharedLog_ struct {
	LsnLogToBeAdded raft.Lsn		// Sequence number of the log to be added
	Entries []LogEntry_
}
func (s *SharedLog_) Init() {
	s.LsnLogToBeAdded = 0
	s.Entries = make([]LogEntry_, 0)
}

// Adds the data into logentry
func (s *SharedLog_) Append(data []byte) (LogEntry_, error) {
	log := LogEntry_{1, s.LsnLogToBeAdded, data, false}
	s.Entries = append(s.Entries, log)
	s.LsnLogToBeAdded++
	return log, nil
}
// Adds the command in the shared log to the input_ch
func (s *SharedLog_) Commit(sequenceNumber raft.Lsn, conn net.Conn) {
	se := r.GetServer(r.id)
	lsnToCommit := se.LsnToCommit
	// Adds the commands to the input_ch to be furthur processed by Evaluator
	for i:=lsnToCommit; i<=sequenceNumber; i++ {
		raft.Input_ch <- raft.String_Conn{string(r.log.Entries[i].Command), conn}
		r.log.Entries[i].IsCommitted = true
	}
	se.LsnToCommit++
}

type AppendRPCArgs struct {
	Entry raft.LogEntry
}
type CommitRPCArgs struct {
	Sequencenumber raft.Lsn
}
type RPC struct {
}

func (a *RPC) AppendRPC(args *AppendRPCArgs, reply *string) error {
	entry := args.Entry
	r.log.Append(entry.Data())
	*reply = "ACK " +strconv.FormatUint(uint64(entry.Lsn()),10)
	log.Print(*reply)
	return nil
}
func (a *RPC) CommitRPC(args *CommitRPCArgs, reply *string) error {
	r.log.Commit(args.Sequencenumber, nil)		// listener: nil - means that it is not supposed to reply back to the client
	*reply = "CACK " +strconv.FormatUint(uint64(args.Sequencenumber),10)
	return nil
}
// ----------------------------------------------

// --------------------------------------
// Raft setup
type ServerConfig struct {
	Id int 				// Id of server. Must be unique
	Hostname string 	// name or ip of host
	ClientPort int 		// port at which server listens to client messages.
	LogPort int 		// tcp port for inter-replica protocol messages.
	isLeader bool		// true, if the server is the leader
	Client *rpc.Client		// Connection object for the server
	LsnToCommit raft.Lsn	// Sequence Number of the last committed log entry
}
type ClusterConfig struct {
	Path string				// Directory for persistent log
	LeaderId int			// ID of the leader
	Servers []ServerConfig	// All servers in this cluster
}
// ----------------------------------------------
// Raft implements the SharedLog interface.
type Raft struct {
	id int
	log SharedLog_
	clusterConfig *ClusterConfig
}
func (r Raft) GetServer(id int) *ServerConfig {
	return &r.clusterConfig.Servers[id]
}

// Initialize the server
func (r *Raft) Init(config *ClusterConfig, thisServerId int) {
	r.id = thisServerId
	r.clusterConfig = config
	r.log.Init()
	go r.AcceptConnection(r.GetServer(r.id).ClientPort)
	go r.AcceptRPC(r.GetServer(r.id).LogPort)
	go raft.Evaluator()
	go AppendCaller()
	go CommitCaller()
	go DataWriter()
}

// Accepts an incoming connection request from the Client, and spawn a ClientListener
func (r *Raft) AcceptConnection(port int) {
	tcpAddr, error := net.ResolveTCPAddr("tcp", "localhost:"+strconv.Itoa(port))

	if error != nil {
		log.Print("[Server] Can not resolve address: ", error)
	}

	ln, err := net.Listen(tcpAddr.Network(), tcpAddr.String())
	if err != nil {
		log.Print("[Server] Error in listening: ", err)
	}
	
	defer ln.Close()
	for {
		// New connection created
		listener, err := ln.Accept()
		if err != nil {
			log.Print("[Server] Error in accepting connection: ", err)
			return
		}
		// Spawn a new listener for this connection
		go r.ClientListener(listener)
	}
}

// Accepts an RPC Request
func (r *Raft) AcceptRPC(port int) {
	// Register this function
	ap1 := new(RPC)
	rpc.Register(ap1)
	gob.Register(LogEntry_{})
	
	listener, e := net.Listen("tcp", "localhost:"+strconv.Itoa(port))
	defer listener.Close()
	if e != nil {
		log.Fatal("[Server] Error in listening:", e)
	}
	for {
	
		conn, err := listener.Accept();
		if err != nil {
			log.Fatal("[Server] Error in accepting: ", err)
		} else {
			go rpc.ServeConn(conn)
		}
	}
}
// ClientListener is spawned for every client to retrieve the command and send it to the input_ch channel
func (r *Raft) ClientListener(listener net.Conn) {
	command, rem := "", ""
	defer listener.Close()
//	log.Print("[Server] Listening on ", listener.LocalAddr(), " from ", listener.RemoteAddr())
	for {
		// Read
		input := make([]byte, 1000)
		listener.SetDeadline(time.Now().Add(3 * time.Second))		// 3 second timeout
		listener.Read(input)
		
		input_ := string(Trim(input))
		if len(input_) == 0 { continue }
		
		// If this is not the leader
		if r.id != r.clusterConfig.LeaderId {
			leader := r.GetServer(r.clusterConfig.LeaderId)
			raft.Output_ch <- raft.String_Conn{"ERR_REDIRECT " + leader.Hostname + " " + strconv.Itoa(leader.ClientPort), listener}
			input = input[:0]
			continue
		}

		command, rem = GetCommand(rem + input_)

		// For multiple commands in the byte stream
		for {
			if command != "" {
				if command[0:3] == "get" {
					raft.Input_ch <- raft.String_Conn{command, listener}
					log.Print(command)
				} else {
	//				log.Print("Command:",command)
					commandbytes := []byte(command)
					// Append in its own log
					l, _ := r.log.Append(commandbytes)
					// Add to the channel to ask everyone to append the entry
					logentry := raft.LogEntry(l)
					raft.Append_ch <- logentry
				
					// ** After getting majority votes
					// Commit in its own store
	//				log.Print(r.GetServer(r.id).LsnToCommit)
					r.log.Commit(r.GetServer(r.id).LsnToCommit, listener)
					// Add to the channel to ask everyone to commit
					raft.Commit_ch <- logentry.Lsn()
				}
			} else { break }
			command, rem = GetCommand(rem)
		}
	}
}
// ----------------------------------------------

// Creates a raft object. This implements the SharedLog interface.
// commitCh is the channel that the kvstore waits on for committed messages.
// When the process starts, the local disk log is read and all committed
// entries are recovered and replayed
func NewRaft(config *ClusterConfig, thisServerId int, commitCh chan raft.LogEntry) (*Raft, error) {
	raft := new(Raft)
	raft.Init(config, thisServerId)
	return raft, nil
}

func ConnectToServers(i int, isLeader bool) {
	// Check for connection unless the ith server accepts the rpc connection
	var client *rpc.Client = nil
	var err error = nil
	// Polls until the connection is made
	for {
		client, err = rpc.Dial("tcp", "localhost:" + strconv.Itoa(9001+2*i))
		if err == nil {
			log.Print("Connected to ", strconv.Itoa(9001+2*i))
			break
		}
	}

	r.clusterConfig.Servers[i] = ServerConfig{
		Id: i,
		Hostname: "Server"+strconv.Itoa(i),
		ClientPort: 9000+2*i, 
		LogPort: 9001+2*i,
		isLeader: isLeader,
		Client: client,
		LsnToCommit: 0,
	}
}
// --------------------------------------------------------------
func main() {
	id, _ := strconv.Atoi(os.Args[1])
	sConfigs := make([]ServerConfig, N)
	for i:=0; i<N; i++ {
		isLeader := false
		if i == 0 { isLeader = true }
		if i != id{		// If it is the leader connecting to every other server
			go ConnectToServers(i, isLeader)
		} else {
			// Its own detail
			sConfigs[i] = ServerConfig{
				Id: i,
				Hostname: "Server"+strconv.Itoa(i),
				ClientPort: 9000+2*i, 
				LogPort: 9001+2*i,
				isLeader: isLeader,
				Client: nil,
				LsnToCommit: 0,
			}
		}
	}
	cConfig := ClusterConfig{"undefined path", 0, sConfigs}
	commitCh := make(chan raft.LogEntry,100)
	r, _ = NewRaft(&cConfig, id, commitCh)

	// Wait until some key is press
	var s string
	fmt.Scanln(&s)
}

