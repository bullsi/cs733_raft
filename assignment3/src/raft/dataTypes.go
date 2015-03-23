package raft
import (
		"time"
		"net"
		"log"
		"net/rpc"
		//~ "encoding/gob"
		"strconv"
)

func Trim(input []byte) []byte {
	i := 0
	for ; input[i] != 0; i++ {}
	return input[0:i]
}

type Lsn uint64 //Log sequence number, unique for all time.
const (
	Follower = 0
	Candidate = 1
	Leader = 2
)
var AllServers []*RaftServer
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
type ServerConfig struct {
	Id int	 				// Id of server. Must be unique
	Hostname string		 	// name or ip of host
	ClientPort int 			// port at which server listens to client messages.
	LogPort int 			// tcp port for inter-replica protocol messages.
	Client *rpc.Client		// Connection object for the server
	LsnToCommit Lsn			// Sequence Number of the last committed log entry
}
type ClusterConfig struct {
	Path string				// Directory for persistent log
	LeaderId int			// ID of the leader
	Servers []ServerConfig	// All servers in this cluster
}

type VoteRequest struct {
	Id				int		// id of the candidate requesting vote
	Term			int
	LastLsn			Lsn
	LastLogTerm		int
}
type VoteReturn struct {
	Term int
	IsVoted bool
}
type AppendRequest struct {
	Id int
	Entry LogEntry
}

// ------------------ RPC ------------------
//~ type RPC struct {
//~ }
//~ type AppendRPCArgs struct {
	//~ Id int
	//~ Entry LogEntry
//~ }
//~ type CommitRPCArgs struct {
	//~ Id int
	//~ Sequencenumber Lsn
//~ }

// ----------------- RaftServer ------------------
type RaftServer struct {
	totalServers	int
	id 				int
	log				SharedLog
	state			int
	
	// --- Vote details -----
	VoteInput_ch 	chan VoteRequest
	VoteOutput_ch 	chan VoteReturn
	Term 			int
	VotedFor		int
	HeartBeatTimer	int
	ElectionTimer	int
	
	// --- Append Log details ----
	AppendInput_ch 	chan AppendRequest			// if the leader wants a follower to add a log, it append the entry to its this channel
	AppendOutput_ch	chan string				// response from the receiver
	CommitInput_ch	chan Lsn
	CommitOutput_ch	chan string
	
	
	clusterConfig 	*ClusterConfig
	KVStore 		map[string]Value
	Input_ch 		chan String_Conn
	Append_ch 		chan LogEntry
	Commit_ch 		chan Lsn
	Output_ch 		chan String_Conn
}


func (r *RaftServer) AppendCaller() {
	for {
		logentry := <-r.Append_ch
		
		// Append this entry to everyone's log
		for i:=0; i<len(r.clusterConfig.Servers); i++ {
			if i == r.id { continue }
			rserv := AllServers[i]
			rserv.AppendInput_ch <- AppendRequest{r.id, logentry}		// r.id is the sender's id
		}
		// Receives the response from everyone concurrently with a timeout
		totalAcks := 0
		numFunc := len(AllServers) - 1
		funcRem := make(chan bool, 5)
		for i:=0; i<len(r.clusterConfig.Servers); i++ {
			if i == r.id { continue }
			go func(id int) {
				var reply string
				rserv := AllServers[id]
				select {
					case reply = <-rserv.AppendOutput_ch:
						totalAcks++
					case <-time.After(1000 * time.Millisecond):
						reply = "TIMEOUT"
				}
				log.Printf("[Server%d %d] %s from %d at port %d", r.id, r.state, reply, id, r.clusterConfig.Servers[id].LogPort)
				funcRem <- true
			}(i)
		}
		for numFunc>0 {
			<-funcRem
			numFunc--
		}
		log.Print("Total Acks: ", totalAcks)
		if totalAcks > len(AllServers)/2 {
			// Got majority of acks, so commit
			r.Commit_ch <- logentry.Lsn()
		}
	}
}
func (r *RaftServer) AppendReceiver() {
	//~ for {
		//~ entry := <-r.AppendInput_ch
		//~ if r.state == Candidate { r.state = Follower }		// Getting append entries while being a candidate
		//~ 
		//~ r.log.Append(entry.Data())
		//~ reply := "ACK " +strconv.FormatUint(uint64(entry.Lsn()),10)
		//~ log.Print("[Server", r.id, "] ", reply, " sent to leader")
		//~ // delay can be added here **
		//~ r.AppendOutput_ch <- reply
	//~ }
}

func (r *RaftServer) CommitCaller() {
	for {
		lsn := <-r.Commit_ch
		// Commit it to everyone's KV Store
		for i:=0; i<len(r.clusterConfig.Servers); i++ {
			if i == r.id { continue }
			rserv := AllServers[i]
			rserv.CommitInput_ch <- lsn
		}
		
		// Does it wait for everyone to send back ACKs that they have committed? **
		//~ for i:=0; i<len(r.clusterConfig.Servers); i++ {
			//~ if i == r.id { continue }
			//~ go func(id int) {
				//~ var reply string
				//~ rserv := AllServers[id]
				//~ select {
					//~ case reply = <-rserv.AppendOutput_ch:
					//~ case <-time.After(1000 * time.Millisecond):
						//~ reply = "TIMEOUT"
				//~ }
				//~ log.Printf("[Server%d] %s from %d at port %d", r.id, reply, id, r.clusterConfig.Servers[id].LogPort)
			//~ }(i)
		//~ }
	}
}

func (r *RaftServer) CommitReceiver() {
	for {
		sequenceNumber := <-r.CommitInput_ch
		r.log.Commit(sequenceNumber, nil)		// listener: nil - means that it is not supposed to reply back to the client
		reply := "CACK " +strconv.FormatUint(uint64(sequenceNumber),10)			// ** reply to commit is not needed
		log.Print("[Server", r.id, "] ", reply, " sent to leader")
		r.CommitOutput_ch <- reply
	}
}

func (r *RaftServer) VoteHandler() {
	//~ for {
		//~ voteReq := <-r.VoteInput_ch
		//~ var term int
		//~ var vote bool
		//~ if r.VotedFor != -1 {					// If it has already voted
			//~ log.Print("[Server", r.id, " ", r.state, "] here0")
			//~ vote = false
		//~ } else if voteReq.Term < r.Term {		// The receiver is already at the newer term
			//~ log.Print("[Server", r.id, " ", r.state, "] here1  ", voteReq.Term, " " ,r.Term)
			//~ term = r.Term
			//~ vote = false
		//~ } else {
			//~ log.Print("[Server", r.id, " ", r.state, "] here2  ", voteReq.Term, " " ,r.Term)
			//~ r.Term = term
			//~ vote = true
		//~ }
		//~ r.VoteOutput_ch<-VoteReturn{vote}
	//~ }
}

func ProcessAppendRequest(r *RaftServer, id int, entry LogEntry) {
	// Set the sender as the leader
	if r.state == Candidate { r.state = Follower }			// Getting append entries while being a candidate
	if entry.Command == nil { return }							// An empty log just as a heartbeat
		
	r.log.Append(r.Term, entry.Data())
	reply := "ACK " +strconv.FormatUint(uint64(entry.Lsn()),10)
	log.Print("[Server", r.id, " ", r.state, "] ", reply, " sent to leader")
	// delay can be added here **
	r.AppendOutput_ch <- reply	
}

func (r *RaftServer) Loop() {
	r.state = Follower
	//~ log.Print("in ")
	for {
		//~ log.Print("in loop")
		for ; r.state == Follower; {
			select {
				// Retain the follower state as long as it is receiving the heartbeats on time
				case entry := <-r.AppendInput_ch:
					//~ log.Print("[Server", r.id, "] Got append request from ", entry.Id)
					ProcessAppendRequest(r, entry.Id, entry.Entry)		// entry.Id = sender's ID
					
				case sequenceNumber := <-r.CommitInput_ch:
					//~ log.Print("[Server", r.id, "] here22")
					r.log.Commit(sequenceNumber, nil)		// listener: nil - means that it is not supposed to reply back to the client
					reply := "CACK " +strconv.FormatUint(uint64(sequenceNumber),10)
					log.Print("[Server", r.id, " ", r.state, "] ", reply, " sent to leader")
					r.CommitOutput_ch <- reply

				case voteReq := <-r.VoteInput_ch:			// Someone ask for vote
					log.Print("[Server", r.id, "] Was asked for vote from ", voteReq.Id, " (", voteReq.Term, " ", r.Term, " ", voteReq.LastLogTerm, " ", r.log.LastTerm, " ", voteReq.LastLsn+1, " ", r.log.LsnLogToBeAdded, ")")
					var term int
					var vote bool
					if voteReq.Term < r.Term {					// The receiver is already at a newer term
						term = r.Term
						vote = false
					} else {
						if r.VotedFor != -1 {					// If it has already voted
							vote = false
						} else if voteReq.LastLogTerm > r.log.LastTerm ||
						(voteReq.LastLogTerm == r.log.LastTerm && voteReq.LastLsn+1 >= r.log.LsnLogToBeAdded) {
							//~ log.Print("[Server", r.id, " ", r.state, "] here2  ", voteReq.Term, " " ,r.Term)
							r.Term = term
							vote = true
						} else {
							term = r.Term
							vote = false
						}
					}
					//~ log.Print("[Server", r.id, " ", r.state, "] voted ", vote)
					r.VoteOutput_ch<-VoteReturn{term, vote}
					
				case <-time.After(time.Duration(r.ElectionTimer) * time.Millisecond):
					//~ log.Print("[Server", r.id, "] here44")
					r.state = Candidate
					log.Print("[Server", r.id,"] changed to Candidate")
					break
			}
		}
		
		for ; r.state == Candidate; {
			r.Term++
			r.VotedFor = r.id
			// Ask for vote from everyone else
			for i:=0; i<len(AllServers); i++ {
				if i == r.id { continue }
				rserv := AllServers[i]
				rserv.VoteInput_ch <- VoteRequest{r.id, r.Term, r.log.LsnLogToBeAdded, r.log.LastTerm}
			}
			// Check their responses concurrently
			totalVotes := 1				// Vote for itself
			numFunc := len(AllServers) - 1
			funcRem := make(chan bool, 5)
			for i:=0; i<len(AllServers); i++ {
				if i == r.id { continue }
				go func(id int){
					rserv := AllServers[id]
					select {
						case vote := <-rserv.VoteOutput_ch:			// vote given by the other server
							if vote.IsVoted {
								totalVotes++
								log.Print("[Server", r.id, "] got vote from ", id)
							}
						case entry := <-rserv.AppendInput_ch:		// append request received from another leader
							ProcessAppendRequest(r, entry.Id, entry.Entry)
							if entry.Entry.Term > r.Term {
								r.state = Follower
								log.Print("[Server", r.id, "] changed + to Follower")
							}
							break
						case <-time.After(1000 * time.Millisecond):
							// loss of vote
					}
					funcRem <- true
				}(i)
			}
			
			for numFunc>0 && totalVotes <= len(AllServers)/2 {
				<-funcRem
				numFunc--
			}
			log.Print("[Server", r.id, "] votes received:", totalVotes)
			
			if totalVotes > len(AllServers)/2 {
				// make it the leader
				r.state = Leader
				log.Print("[Server", r.id, "] changed to Leader")
				break
			} else {
				// what if the candidate does not receive majority of votes **
				// lets say it becomes a follower again
				r.state = Follower
				log.Print("[Server", r.id, "] changed to Follower")
			}
		}

		if r.state == Leader {
			log.Print("[Server", r.id, "] Leader sending heartbeats...")
			// send an empty append request as a heartbeat
			time.Sleep(500 * time.Millisecond)
			for i:=0; i<len(AllServers); i++ {
				if i == r.id { continue }
				rserv := AllServers[i]
				var dummy LogEntry
				dummy.Term = rserv.Term
				dummy.Command = nil
				rserv.AppendInput_ch <- AppendRequest{r.id, dummy}		// r.id = sender's ID... i = receiver's ID
			}
			break
		}
	}
}


func (r RaftServer) GetServer(id int) *ServerConfig {
	return &r.clusterConfig.Servers[id]
}

// Accepts an incoming connection request from the Client, and spawn a ClientListener
func (r *RaftServer) AcceptConnection(port int) {
	tcpAddr, error := net.ResolveTCPAddr("tcp", "localhost:"+strconv.Itoa(port))

	if error != nil {
		log.Printf("[Server%d] Can not resolve address: %s", r.id, error)
	}

	ln, err := net.Listen(tcpAddr.Network(), tcpAddr.String())
	if err != nil {
		log.Printf("[Server%d] Error in listening: %s", r.id, err)
	}
	
	defer ln.Close()
	for {
		// New connection created
		listener, err := ln.Accept()
		if err != nil {
			log.Printf("[Server%d] Error in accepting connection: %s", r.id, err)
			return
		}
		// Spawn a new listener for this connection
		go r.ClientListener(listener)
	}
}

// Accepts an RPC Request
//~ func (r *RaftServer) AcceptRPC(port int) {
	//~ // Register this function
	//~ ap1 := new(RPC)
	//ap1.r = r
	//~ rpc.Register(ap1)
	//rpc.Register(r)
	//~ 
	//~ gob.Register(LogEntry{})
	//~ 
	//~ listener, e := net.Listen("tcp", "localhost:"+strconv.Itoa(port))
	//~ defer listener.Close()
	//~ if e != nil {
		//~ log.Fatal("[Server] Error in listening:", e)
	//~ }
	//~ for {
	//~ 
		//~ conn, err := listener.Accept();
		//~ if err != nil {
			//~ log.Fatal("[Server] Error in accepting: ", err)
		//~ } else {
			//~ go rpc.ServeConn(conn)
		//~ }
	//~ }
//~ }

// ClientListener is spawned for every client to retrieve the command and send it to the input_ch channel
func (r *RaftServer) ClientListener(listener net.Conn) {
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
		if r.state != Leader {
			leader := r.GetServer(r.clusterConfig.LeaderId)
			r.Output_ch <- String_Conn{"ERR_REDIRECT " + leader.Hostname + " " + strconv.Itoa(leader.ClientPort), listener}
			input = input[:0]
			continue
		}

		command, rem = r.GetCommand(rem + input_)
		// For multiple commands in the byte stream
		for {
			if command != "" {
				if command[0:3] == "get" {
					r.Input_ch <- String_Conn{command, listener}
					log.Printf("[Server%d %d] %s", r.id, r.state, command)
				} else {
	//				log.Print("Command:",command)
					commandbytes := []byte(command)
					// Append in its own log
					l, _ := r.log.Append(r.Term, commandbytes)
					// Add to the channel to ask everyone to append the entry
					logentry := LogEntry(l)				// typecasting
					r.Append_ch <- logentry
				}
			} else { break }
				command, rem = r.GetCommand(rem)
		}
	}
}
// Connect to other servers
func (r *RaftServer) ConnectToServers(i int, isLeader bool) {
	// Check for connection unless the ith server accepts the rpc connection
	var client *rpc.Client = nil
	var err error = nil
	// Polls until the connection is made
	for {
		client, err = rpc.Dial("tcp", "localhost:" + strconv.Itoa(9001+2*i))
		if err == nil {
			log.Print("[Server", r.id, "] Connected to ", strconv.Itoa(9001+2*i))
			break
		}
	}
	r.clusterConfig.Servers[i] = ServerConfig{
		Id: i,
		Hostname: "Server"+strconv.Itoa(i),
		ClientPort: 9000+2*i, 
		LogPort: 9001+2*i,
		//~ IsLeader: isLeader,
		Client: client,
		LsnToCommit: 0,
	}
}

// Initialize the server
func (r *RaftServer) Init(totalServers int, config *ClusterConfig, thisServerId int) {
	r.totalServers = totalServers
	r.id = thisServerId
	r.clusterConfig = config
	r.log.Init(r)
	
	r.KVStore = make(map[string]Value)
	r.Input_ch = make(chan String_Conn, 10000)
	r.Append_ch = make(chan LogEntry, 10000)
	r.AppendInput_ch = make(chan AppendRequest, 10000)
	r.AppendOutput_ch = make(chan string, 10000)
	
	r.Commit_ch = make(chan Lsn, 10000)
	r.CommitInput_ch = make(chan Lsn, 10000)
	r.CommitOutput_ch = make(chan string, 10000)
	r.Output_ch = make(chan String_Conn, 10000)
	
	// --- Vote details ---
	r.VoteInput_ch = make(chan VoteRequest, totalServers + 2)		// + 2 just to be on a safe side (as of now)
	r.VoteOutput_ch = make(chan VoteReturn, totalServers + 2)
	r.Term = 1
	r.VotedFor = -1			// voted for no one
	r.HeartBeatTimer = 1000
	r.ElectionTimer = 1000
	
	go r.AcceptConnection(r.GetServer(r.id).ClientPort)
	//~ go r.AcceptRPC(r.GetServer(r.id).LogPort)
	go r.Evaluator()
	go r.AppendCaller()
	go r.AppendReceiver()
	go r.CommitCaller()
	go r.CommitReceiver()
	go r.DataWriter()
	go r.VoteHandler()
	go r.Loop()
	
	// Connect to other servers and store their details
	for i:=0; i<totalServers; i++ {
		isLeader := false
		if i == 0 { isLeader = true }
		if i != r.id {
			go r.ConnectToServers(i, isLeader)
		}
	}
}

// ------------- LogEntry -------------------
type LogEntry struct {
	Term int
	SequenceNumber Lsn
	Command []byte
	IsCommitted bool
}
func (l LogEntry) Lsn() Lsn {
	return l.SequenceNumber
}
func (l LogEntry) Data() []byte {
	return l.Command
}
func (l LogEntry) Committed() bool {
	return l.IsCommitted
}

// ------------- SharedLog -------------------
type SharedLog struct {
	LsnLogToBeAdded Lsn		// Sequence number of the log to be added
	LastTerm int			// Term of the last log added
	Entries []LogEntry
	r *RaftServer
}
func (s *SharedLog) Init(r *RaftServer) {
	s.LsnLogToBeAdded = 0
	s.Entries = make([]LogEntry, 0)
	s.r = r
	s.LastTerm = -1
}
// Adds the data into logentry
func (s *SharedLog) Append(term int, data []byte) (LogEntry, error) {
	log := LogEntry{term, s.LsnLogToBeAdded, data, false}
	s.Entries = append(s.Entries, log)
	s.LsnLogToBeAdded++
	s.LastTerm = term
	return log, nil
}
// Adds the command in the shared log to the input_ch
func (s *SharedLog) Commit(sequenceNumber Lsn, conn net.Conn) {
	se := s.r.GetServer(s.r.id)
	lsnToCommit := se.LsnToCommit
	// Adds the commands to the input_ch to be furthur processed by Evaluator
	for i:=lsnToCommit; i<Lsn(len(s.Entries)) && i<=sequenceNumber; i++ {
		s.r.Input_ch <- String_Conn{string(s.Entries[i].Command), conn}
		s.Entries[i].IsCommitted = true
	}
	se.LsnToCommit++
}


// ------------- RPC -------------------
//~ func (a *RPC) AppendRPC(args *AppendRPCArgs, reply *string) error {
	//~ r := AllServers[args.Id]
	//~ entry := args.Entry
	//~ r.log.Append(entry.Data())
	//~ *reply = "ACK " +strconv.FormatUint(uint64(entry.Lsn()),10)
	//~ log.Print("[Server", r.id, "] ", *reply, " sent to leader")
	//~ return nil
//~ }
//~ func (a *RPC) CommitRPC(args *CommitRPCArgs, reply *string) error {
	//~ r := AllServers[args.Id]
	//~ r.log.Commit(args.Sequencenumber, nil)		// listener: nil - means that it is not supposed to reply back to the client
	//~ *reply = "CACK " +strconv.FormatUint(uint64(args.Sequencenumber),10)
	//~ return nil
//~ }
