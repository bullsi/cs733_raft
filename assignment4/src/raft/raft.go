package raft
import (
		"fmt"
		"time"
		"net"
		"log"
		"net/rpc"
		//~ "encoding/gob"
		"strconv"
)

// Trims a byte array from the point where '\0' is found
func Trim(input []byte) []byte {
	i := 0
	for ; input[i] != 0; i++ {}
	return input[0:i]
}

type Lsn int64 //Log sequence number, unique for all time.
const (
	Follower = 0
	Candidate = 1
	Leader = 2
)

var StartHeartBeatChannel chan bool
var AllServers []*RaftServer
var Server *RaftServer

type Value struct {
	Text			string
	ExpiryTime		time.Time
	IsExpTimeInf	bool
	NumBytes		int
	Version			int64
}
type String_Conn struct {
	Text 		string
	Conn		net.Conn
}
type LogEntry_Conn struct {
	Entry		LogEntry
	Conn		net.Conn
}
type Lsn_Conn struct {
	SequenceNumber	Lsn
	Conn			net.Conn
}
type ServerConfig struct {
	Id int	 				// Id of server. Must be unique
	Hostname string		 	// name or ip of host
	ClientPort int 			// port at which server listens to client messages.
	LogPort int 			// tcp port for inter-replica protocol messages.
	Client *rpc.Client		// Connection object for the server
	NextIndex Lsn			// Log index of the next entry to be sent to the follower
	
	// --- Append IO details --- (Leader keeps track of data sent/received to other servers)
	AppendInput 	*AppendRequest			// input to AppendRPC
	AppendOutput	AppendResponse			// output of AppendRPC
	AppendRPCErr_ch	chan error				// When the AppendRPC call returns, this channel gets a signal
	CommitInput		*Lsn
	CommitOutput	string
	CommitRPCErr_ch	chan error
	
	// --- Vote IO details ---
	VoteInput		*VoteRequest
	VoteOutput		VoteResponse
	VoteRPCErr_ch	chan error
}
type ClusterConfig struct {
	Path string				// Directory for persistent log
	LeaderId int			// ID of the leader
	Servers []ServerConfig	// All servers in this cluster
}

type VoteRequest struct {
	Id				int		// Id of the candidate requesting vote
	Term			int		// Term of the election
	LastLsn			Lsn		// Lsn (Log Sequence Number) of the latest log entry
	LastLogTerm		int		// Term of the latest log entry
}
type VoteResponse struct {
	Term int				// Term for it has voted
	IsVoted bool			// Voted or not
}
type AppendRequest struct {
	Id				int			// Id of the server asking to append
	//~ Entry			LogEntry	// 	
	Term			int
	SequenceNumber	Lsn
	Command			[]byte
	PrevTerm		int			//
}
type AppendResponse struct {
	HasAccepted		bool		// True, if the follower has accepted the packet
	SequenceNumber	Lsn			// Sequence number of log entry sent by the server (needed, to know the log entry to which this is the response)
	Term			int			// Follower's current term (to update the server, in case it is at a previous term)
}

// ----------------- RaftServer ------------------
type RaftServer struct {
	totalServers	int
	id 				int
	log				SharedLog
	state			int
	LsnToCommit		Lsn							// next index to commit, all the logs before this Lsn are committed
	
	// --- Vote details -----
	VoteInput_ch 	chan VoteRequest
	VoteOutput_ch 	chan VoteResponse
	Term 			int
	VotedFor		int
	HeartBeatTimer	int
	ElectionTimer	int
	
	// --- Append Requests from the leader are added to the channel ----
	AppendInput_ch 	chan AppendRequest			// from RPC to this channel
	AppendOutput_ch	chan AppendResponse			// from this channel back to RPC return
	CommitInput_ch	chan Lsn
	CommitOutput_ch	chan string
	
	clusterConfig 	*ClusterConfig
	KVStore 		map[string]Value
	Append_ch 		chan bool					// just to notify that an append is needed
	Commit_ch 		chan Lsn
	Input_ch 		chan String_Conn
	Output_ch 		chan String_Conn
}

func (r *RaftServer) WaitForAppendResponse(id int) {
	if r.state == Leader {
		rserv := &r.clusterConfig.Servers[id]
		select {
			case pp := <-rserv.AppendRPCErr_ch:
				reply := rserv.AppendOutput
				//~ log.Print("[Server", r.id, "] Received ", reply, " from ", id, "    ", pp)
				if pp != nil {
					rserv.Client = nil
					return
				}
				if reply.HasAccepted == false {
					// not appended
					// decrease NextIndex for id'th server
					// But what if the follower rejects even the first packet (invalid case though, but handled)
					if reply.SequenceNumber == 0 { rserv.NextIndex = 0
					} else { rserv.NextIndex = reply.SequenceNumber - 1 }
					
					//~ index := r.clusterConfig.Servers[id].NextIndex
					//~ logentry_ := r.log.Entries[index]
					//~ prevterm := -1										// term of the previous log entry
					//~ if index>0 { prevterm = r.log.Entries[index-1].Term }
					// send NextIndex'th log entry
					//~ log.Print("prev rejected. asked ", id, " to append log: ", logentry_)
					//~ rserv.AppendInput = &AppendRequest{r.id, logentry_.Term, logentry_.SequenceNumber, logentry_.Command, prevterm}
					//~ rserv.AppendRPCErr_ch <- rserv.Client.Call("RPC.AppendRPC", rserv.AppendInput, &rserv.AppendOutput)
					//~ log.Print("[Server",r.id,"] + received --> ", rserv.AppendOutput)
					r.WaitForAppendResponse(id)
					r.Append_ch <- true					// trigger to send the next packets
				} else {
					// search that log in r.log and set its Acks value accordingly
					seq := reply.SequenceNumber
					
					log_ := r.log.GetLog(seq)
					log_.Acks |= (1<<uint(id))
					rserv.NextIndex = seq+1
					// check if the entry is needed to be committed.
					countAcks := 0
					for j:=0; j<r.totalServers; j++ {
						if (log_.Acks>>uint(j)) & 1 == 1 {
							countAcks++
						}
					}
					// if majority of acks received and prev entry is committed
					prevlog := r.log.GetLog(seq-1)
					//~ log.Print("[Server", r.id, "] Total Acks: ", countAcks, "/", r.totalServers/2+1, " seq: ", seq, " prev: ", prevlog)
					// If
					// Received enough ACKs
					// This entry is not committed
					// Previous entry is committed
					if (countAcks == r.totalServers/2) && (seq==0 || prevlog.IsCommitted) {
						//~ log.Print("Added to commit channel")
						r.Commit_ch <- log_.SequenceNumber
						// check for later entries
					}
					//~ r.ShowLog()
				}
			case <-time.After(1000 * time.Millisecond):
				// timeout
		}
	}
}

// This function 
func (r *RaftServer) AppendCaller() {
	for {
		<-r.Append_ch							// a new entry has been appended
		//~ log.Print("append")
		// Append this entry to everyone's log
		for i:=0; i<len(r.clusterConfig.Servers); i++ {
			if i == r.id { continue }
			go func(id int) {
				rserv := &r.clusterConfig.Servers[id]
				if rserv.Client != nil {			// if the server is connected
					//~ log.Print(rserv.NextIndex, " - ")
					l := r.log.GetLog(rserv.NextIndex)
					prevterm := -1
					if l.SequenceNumber > 0 {
						prevterm = r.log.GetLog(rserv.NextIndex - 1).Term
					}
					rserv.AppendInput = &AppendRequest{r.id, l.Term, l.SequenceNumber, l.Command, prevterm}
					//~ log.Print("[Server", r.id,"] asked ",id ," to append log: ", l)
					rserv.AppendOutput = AppendResponse{}
					rserv.AppendRPCErr_ch <- rserv.Client.Call("RPC.AppendRPC", rserv.AppendInput, &rserv.AppendOutput)
					//~ log.Print("[Server",r.id,"] received --> ", rserv.AppendOutput, " from ", id)
				}
			}(i)
		}
		
		// Receives responses from everyone concurrently with a timeout
		for i:=0; i<len(r.clusterConfig.Servers); i++ {
			if i == r.id { continue }
			r.WaitForAppendResponse(i)
		}
	}
}

func (r *RaftServer) CommitCaller() {
	for {
		lsn := <-r.Commit_ch

		// Evaulate and Commit till lsn.SequenceNumber
		r.log.Commit(lsn)
		// Ask every follower to commit
		for i:=0; i<len(r.clusterConfig.Servers); i++ {
			if i == r.id { continue }
			rserv := &r.clusterConfig.Servers[i]
			go func(rserv *ServerConfig) {
				if rserv.Client != nil {			// if the server is not connected yet
					rserv.CommitInput = &lsn
					//~ log.Print("asked to commit: ", *rserv.CommitInput)
					rserv.CommitRPCErr_ch <- rserv.Client.Call("RPC.CommitRPC", rserv.CommitInput, &rserv.CommitOutput)
					//~ log.Print("[Server",r.id,"] commit received (doesn't matter) --> ", rserv.CommitOutput)
				}
			}(rserv)
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

func (r *RaftServer) ProcessAppendRequest(id int, Term int, SequenceNumber Lsn, Command []byte, prevTerm int) {
	// Set the sender as the leader
	r.clusterConfig.LeaderId = id
	if r.state == Candidate { r.state = Follower }				// Getting append entries while being a candidate
	if Command == nil { return }							// An empty log just as a heartbeat
	
	log.Print("{", Term, SequenceNumber, r.log.LsnLogToBeAdded, "}")
	// ** need to match the terms of leader and follower
	hasaccepted := false				// True, if it has accepted the log entry
	sequencenumber := SequenceNumber	// Sequence number for which this follower will respond
	term := r.Term						// Follower's term to be returned
	
	if r.Term < Term {		// r.Term is not the latest, don't accept the packet but update r.Term
		r.Term = Term
	} else if r.Term > Term {
		// If the follower's term is the latest, don't accept the packet and update the server with the latest term
		// hasaccepted = false
		// term = r.Term
	} else if SequenceNumber <= r.log.LsnLogToBeAdded {		// if the incoming log number is not higher than what the follower expects next
		if SequenceNumber==0 || r.log.Entries[SequenceNumber-1].Term == prevTerm {		// check if the terms of the previous logs match
			// slice the log from entry.Lsn() onwards
			r.log.Entries = r.log.Entries[:SequenceNumber]			// most of the time it would be 
			// append the new entry
			r.log.Append(Term, Command, nil)
			hasaccepted = true
		}
	}
	//~ log.Print("[Server", r.id, "] Sent HasAccepted:", hasaccepted, " Lsn:", sequencenumber, " Term:", term, " to leader")
	// delay can be added here **
	r.AppendOutput_ch <- AppendResponse{hasaccepted, sequencenumber, term}
}

func (r *RaftServer) FollowerLoop() {
	//~ for {
		r.VotedFor = -1
		for r.state == Follower {
			select {
				// Retain the follower state as long as it is receiving the heartbeats on time
				case entry := <-r.AppendInput_ch:
					//~ log.Print("[Server", r.id, "] Got append request from ", entry.Id, " comm: ", entry.Command)
					r.ProcessAppendRequest(entry.Id, entry.Term, entry.SequenceNumber, entry.Command, entry.PrevTerm)		// entry.Id = sender's ID
					//~ r.ShowLog()
					break
				case sequenceNumber := <-r.CommitInput_ch:
					//~ log.Print("[Server", r.id, "] here22")
					r.log.Commit(sequenceNumber)		// listener: nil - means that it is not supposed to reply back to the client
					reply := "CACK " +strconv.FormatUint(uint64(sequenceNumber),10)
					//~ log.Print("[Server", r.id, " ", r.state, "] ", reply, " sent to leader")
					r.CommitOutput_ch <- reply
					break
				case voteReq := <-r.VoteInput_ch:				// Someone asked for vote
					//~ log.Print("[Server", r.id, "] was asked for vote: ", voteReq, " but it has already voted to ", r.VotedFor, " with term ", r.Term)
					var term int
					var vote bool
					if voteReq.Term < r.Term {					// The receiver is already at a newer term
						term = r.Term
						vote = false
					} else {
						if voteReq.Term == r.Term && r.VotedFor != -1 {					// If it has already voted
							vote = false
						} else if voteReq.LastLogTerm > r.log.LastTerm ||
						(voteReq.LastLogTerm == r.log.LastTerm && voteReq.LastLsn+1 >= r.log.LsnLogToBeAdded) {
							//~ log.Print("[Server", r.id, " ", r.state, "] here2  ", voteReq.Term, " " ,r.Term)
							r.Term = voteReq.Term
							vote = true
							r.VotedFor = voteReq.Id
						} else {
							term = r.Term
							vote = false
						}
					}
					//~ log.Print("[Server", r.id,"] voted ", vote)
					r.VoteOutput_ch<-VoteResponse{term, vote}
					break

				case <-time.After(time.Duration(r.ElectionTimer) * time.Millisecond):
					//~ log.Print("[Server", r.id, "] here44")
					r.state = Candidate
					log.Print("[Server", r.id,"] changed to Candidate")
					break
			}
		}
	//~ }
}

func (r *RaftServer) CandidateLoop() {
	r.Term++
	r.VotedFor = r.id
	for r.state == Candidate {
		// Ask for vote from everyone else
		for i:=0; i<len(r.clusterConfig.Servers); i++ {
			if i == r.id { continue }
			rserv := &r.clusterConfig.Servers[i]
			go func(rserv *ServerConfig) {
				if rserv.Client != nil {			// if the server is connected yet
					rserv.VoteInput = &VoteRequest{r.id, r.Term, r.log.LsnLogToBeAdded-1, r.log.LastTerm}
					//~ log.Print("[Server",r.id,"] asked ", rserv.Id, " for vote: ", rserv.VoteInput)
					rserv.VoteRPCErr_ch <- rserv.Client.Call("RPC.VoteRPC", rserv.VoteInput, &rserv.VoteOutput)
					//~ log.Print("[Server",r.id,"] received ", rserv.VoteOutput)
				}
			}(rserv)
		}
		
		// Check their responses concurrently
		totalVotes := 1				// Vote for itself
		numFunc := len(r.clusterConfig.Servers) - 1
		funcRem := make(chan bool, 5)
		for i:=0; i<len(r.clusterConfig.Servers); i++ {
			if i == r.id { continue }
			go func(id int){
				rserv := &r.clusterConfig.Servers[id]
				select {
					case voteReq := <-r.VoteInput_ch:				// Someone asked for vote
						//~ log.Print("[Server", r.id, "] was asked for vote from ", voteReq.Id, " but it has already voted to ", r.VotedFor)
						if voteReq.Term > r.Term {					// The receiver is already at a newer term
							// change to follower if term is higher
							r.state = Follower
							log.Print("[Server", r.id, "] changed + to Follower")
						}
						break
					case <-rserv.VoteRPCErr_ch:
						vote := rserv.VoteOutput						// vote given by the other server
						if vote.IsVoted {
							totalVotes++
							//~ log.Print("[Server", r.id, "] got vote: ", rserv.VoteOutput, " from ", rserv.Id)
						}
						break
					case <-rserv.AppendRPCErr_ch:
						entry := rserv.AppendInput
						r.ProcessAppendRequest(entry.Id, entry.Term, entry.SequenceNumber, entry.Command, entry.PrevTerm)		// entry.Id = sender's ID
						if entry.Term > r.Term {
							r.state = Follower
							log.Print("[Server", r.id, "] changed + to Follower")
						}
						break
					case <-time.After(1000 * time.Millisecond):
						//~ log.Print("timeout")
						// loss of vote
				}
				funcRem <- true
			}(i)
		}
		if r.state == Follower { return }
		
		// wait for all go routines to finish
		for numFunc>0 && totalVotes <= len(AllServers)/2 {
			<-funcRem
			numFunc--
		}
		log.Print("[Server", r.id, "] Votes received: ", totalVotes)
		
		if totalVotes > len(r.clusterConfig.Servers)/2 {
			// make it the leader
			r.state = Leader
			log.Print("[Server", r.id, "] changed to Leader")
			
			/*
			// add some dummy log entries
			data := []byte{100,49}
			r.log.Append(r.Term, data, nil)
			data = []byte{100,50}
			r.log.Append(r.Term, data, nil)
			//~ data = []byte{100,51}
			//~ r.log.Append(1, data, nil)
			//~ data = []byte{100,52}
			//~ r.log.Append(1, data, nil)
			
			// after becoming the leader, it thinks that the followers are as updated as itself
			for id:=0; id<r.totalServers; id++ {
				if id == r.id { continue }
				rserv := &r.clusterConfig.Servers[id]
				rserv.NextIndex = r.log.LsnLogToBeAdded-1
			}
			r.Append_ch <- true			// just a trigger
			*/
		} else {
			// what if the candidate does not receive majority of votes **
			// lets say it becomes a follower again
			r.state = Follower
			log.Print("[Server", r.id, "] changed to Follower")
		}
	}
}

// Listens for a vote/append request with a higher term
func (r *RaftServer) LeaderLoop() {
	if r.state == Leader {
		select {
			case voteReq := <-r.VoteInput_ch:			// someone asked for vote
				for i:=0; i<len(r.clusterConfig.Servers); i++ {
					if i == r.id { continue }
					go func(id int){
						//~ rserv := &r.clusterConfig.Servers[id]
						log.Print("[Server", r.id, "] was asked for vote from ", voteReq.Id, " with term ", voteReq.Term)
						if voteReq.Term > r.Term {
							// change to follower if term is higher
							r.state = Follower
							log.Print("[Server", r.id, "] changed + to Follower")
							//~ return
						}
					}(i)
				}
				break
			// Receiving from append channel
			// UNCOMMENT THE FOLLOWING AND COMMENT "go AppendCaller()" and "go CommitCaller()"
			/*case <-r.Append_ch:							// a new entry has been appended
				//~ log.Print("append")
				// Append this entry to everyone's log
				for i:=0; i<len(r.clusterConfig.Servers); i++ {
					if i == r.id { continue }
					go func(id int) {
						rserv := &r.clusterConfig.Servers[id]
						if rserv.Client != nil {			// if the server is connected
							log.Print(rserv.NextIndex, " - ")
							l := r.log.GetLog(rserv.NextIndex)
							prevterm := -1
							if l.SequenceNumber > 0 {
								prevterm = r.log.GetLog(rserv.NextIndex - 1).Term
							}
							rserv.AppendInput = &AppendRequest{r.id, l.Term, l.SequenceNumber, l.Command, prevterm}
							log.Print("[Server", r.id,"] asked ",id ," to append log: ", l)
							rserv.AppendOutput = AppendResponse{}
							rserv.AppendRPCErr_ch <- rserv.Client.Call("RPC.AppendRPC", rserv.AppendInput, &rserv.AppendOutput)
							//~ log.Print("[Server",r.id,"] received --> ", rserv.AppendOutput, " from ", id)
						}
					}(i)
				}
				// Receives responses from everyone concurrently with a timeout
				for i:=0; i<len(r.clusterConfig.Servers); i++ {
					if i == r.id { continue }
					r.WaitForAppendResponse(i)
				}
				break
			// Receving from commit channel
			case lsn := <-r.Commit_ch:
				// Evaulate and Commit till lsn.SequenceNumber
				r.log.Commit(lsn)
				// Ask every follower to commit
				for i:=0; i<len(r.clusterConfig.Servers); i++ {
					if i == r.id { continue }
					rserv := &r.clusterConfig.Servers[i]
					go func(rserv *ServerConfig) {
						if rserv.Client != nil {			// if the server is not connected yet
							rserv.CommitInput = &lsn
							//~ log.Print("asked to commit: ", *rserv.CommitInput)
							rserv.CommitRPCErr_ch <- rserv.Client.Call("RPC.CommitRPC", rserv.CommitInput, &rserv.CommitOutput)
							//~ log.Print("[Server",r.id,"] commit received (doesn't matter) --> ", rserv.CommitOutput)
						}
					}(rserv)
				}
				break
				*/
		}
		
		// Send heartbeat after a certain timeout
		//~ time.Sleep(500 * time.Millisecond)
		//~ case <-time.After(500 * time.Millisecond):
	}
}

func (r *RaftServer) LeaderLoopHeartbeat() {
	//~ for {
		//~ <-StartHeartBeatChannel
		if r.state == Leader {
			// send an empty append request as a heartbeat
			time.Sleep(500 * time.Millisecond)
			//~ log.Print("[Server", r.id, "] Leader sending heartbeats...")
			for i:=0; i<len(r.clusterConfig.Servers); i++ {
				if i == r.id { continue }
				var dummy LogEntry
				dummy.Term = r.Term
				dummy.SequenceNumber = -1
				dummy.Command = nil
				
				go func(id int) {
					rserv := &r.clusterConfig.Servers[id]
					if rserv.Client != nil {			// if the server is connected
						rserv.AppendInput = &AppendRequest{r.id, dummy.Term, dummy.SequenceNumber, dummy.Command, -1}			// r.id = sender's ID... i = receiver's ID
						//~ log.Print("[Server",r.id,"] asked ",id ," to append log+: ", dummy, "  <heartbeatIn>)
						//~ heartbeatout := AppendResponse{}
						rserv.AppendRPCErr_ch <- rserv.Client.Call("RPC.AppendRPC", rserv.AppendInput, &rserv.AppendOutput)
						//~ log.Print("[Server",r.id,"] received+ --> ", rserv.AppendOutput, " from ", id, " <heartbeatOut>")
						
						// Sometimes this receives the response of the actual append message
						//~ go r.WaitForAppendResponse(id)
					}
				}(i)
			}
			//~ r.ShowLog()
			//~ log.Print("[Server", r.id, "] Heartbeats sent")
			//~ break
		}
	//~ }
}

func (r *RaftServer) Loop() {
	r.state = Follower
	for {
		if r.state == Follower {
			r.FollowerLoop()
		}
		if r.state == Candidate {
			r.CandidateLoop()
		}
		if r.state == Leader {
			//~ if len(StartHeartBeatChannel)==0 { StartHeartBeatChannel <- true }
			//~ r.LeaderLoop()
			r.LeaderLoopHeartbeat()
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

func (r *RaftServer) AcceptRPC(port int) {
	// Register this function
	ap1 := new(RPC)
	rpc.Register(ap1)
	//~ gob.Register(LogEntry_{})
	
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
			//~ defer conn.Close()
		}
	}
}

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
			log.Print(r.clusterConfig.LeaderId, " is the leader. Redirect.")
			leader := r.GetServer(r.clusterConfig.LeaderId)
			r.Output_ch <- String_Conn{"ERR_REDIRECT " + leader.Hostname + " " + strconv.Itoa(leader.ClientPort), listener}
			input = input[:0]
			continue
		}

		command, rem = r.GetCommand(rem + input_)
		// For multiple commands in the byte stream
		for {
			//~ log.Print("input: ", command)
			if command != "" {
				if command[:3] == "get" {
					r.Input_ch <- String_Conn{command, listener}
					//~ log.Printf("[Server%d] %s", r.id, command)
				} else {
					//~ log.Print("Command:",command[0:10])
					commandbytes := []byte(command)
					// Append in its own log
					r.log.Append(r.Term, commandbytes, listener)
					// Add to the channel to ask everyone to append the entry
					r.Append_ch <- true
				}
			} else { break }
				command, rem = r.GetCommand(rem)
		}
	}
}

// Displays the log [{term,lsn}...]
func (r *RaftServer) ShowLog() {
	fmt.Print("[Server", r.id, "] Log: ")
	for i:=0; i<len(r.log.Entries) /*r.log.LsnLogToBeAdded -1*/; i++ {
		fmt.Print("{", r.log.Entries[i].Term, r.log.Entries[i].Lsn(), r.log.Entries[i].IsCommitted, "}, ")
	}
	fmt.Println()
}

// Initialize the server
func (r *RaftServer) Init(totalServers int, config *ClusterConfig, thisServerId int) {
	r.totalServers = totalServers
	r.id = thisServerId
	r.state = Follower
	r.clusterConfig = config
	r.log.Init(r)
	
	r.KVStore = make(map[string]Value)
	r.Input_ch = make(chan String_Conn, 10000)
	r.Append_ch = make(chan bool, 10000)
	r.AppendInput_ch = make(chan AppendRequest, 10000)
	r.AppendOutput_ch = make(chan AppendResponse, 10000)
	
	r.Commit_ch = make(chan Lsn, 10000)
	r.CommitInput_ch = make(chan Lsn, 10000)
	r.CommitOutput_ch = make(chan string, 10000)
	r.Output_ch = make(chan String_Conn, 10000)
	
	// --- Vote details ---
	r.VoteInput_ch = make(chan VoteRequest, totalServers + 2)		// + 2 just to be on a safe side (as of now)
	r.VoteOutput_ch = make(chan VoteResponse, totalServers + 2)
	r.Term = 0
	r.VotedFor = -1				// voted for no one
	r.HeartBeatTimer = 1000
	r.ElectionTimer = 1000		+ 5000 * r.id
	
	go r.AcceptConnection(r.GetServer(r.id).ClientPort)
	go r.AcceptRPC(r.GetServer(r.id).LogPort)
	go r.Evaluator()
	go r.AppendCaller()
	go r.CommitCaller()
	go r.DataWriter()
	go r.Loop()
	
	//~ go r.LeaderLoopHeartbeat()

	for i:=0; i<totalServers; i++ {
		if i!=r.id {
			go r.ConnectToServers(i)
		}
	}
}

func (r *RaftServer) ConnectToServers(i int) {
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

	r.clusterConfig.Servers[i] = ServerConfig {
		Id: i,
		Hostname: "Server"+strconv.Itoa(i),
		ClientPort: 9000+2*i, 
		LogPort: 9001+2*i,
		Client: client,
		NextIndex: 0, //r.log.LsnLogToBeAdded-1,
		AppendRPCErr_ch: make(chan error, 10),
		VoteRPCErr_ch: make(chan error, 10),
	}

	//~ log.Println("---->", r.log.LsnLogToBeAdded)
	rserv := &r.clusterConfig.Servers[i]
	next := rserv.NextIndex	
	//~ prevterm := -1
	if next > 0 {
		//~ prevterm = r.log.Entries[next - 1].Term
	}
	if next >= 0 {
		//~ l := r.log.Entries[next]
		//~ rserv.AppendInput = &AppendRequest{r.id, l.Term, l.SequenceNumber, l.Command, prevterm}
		//~ rserv.AppendRPCErr_ch <- rserv.Client.Call("RPC.AppendRPC", rserv.AppendInput, &rserv.AppendOutput)
		//~ r.WaitForAppendResponse(i, l.Client)
	} else {
		rserv.NextIndex = 0
	}
}

// ------------- LogEntry -------------------
type LogEntry struct {
	Term			int
	SequenceNumber	Lsn
	Command			[]byte
	IsCommitted		bool
	Acks			int			// stores in a bitwise manner which followers have received the append request
	Client			net.Conn
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
	Entries []LogEntry		// Entries
	r *RaftServer			// Server to which this log belongs to
}
func (s *SharedLog) Init(r *RaftServer) {
	s.LsnLogToBeAdded = 0
	s.Entries = make([]LogEntry, 0)
	s.r = r
	s.LastTerm = -1
}
// Adds the data into logentry
func (s *SharedLog) Append(term int, data []byte, conn net.Conn) (LogEntry, error) {
	//~ log := LogEntry{term, s.LsnLogToBeAdded, data, false, 0}
	log := LogEntry{term, s.LsnLogToBeAdded, data, false, 1<<uint(s.r.id), conn}
	if s.LsnLogToBeAdded == Lsn(len(s.Entries)) {
		s.Entries = append(s.Entries, log)
		//~ fmt.Println("[Server", s.r.id, "] ", Lsn((len(s.Entries))), "--->")
	} else if s.LsnLogToBeAdded < Lsn(len(s.Entries)) {
		fmt.Print("[Server", s.r.id, "] ", s.LsnLogToBeAdded, " ", Lsn(len(s.Entries)))
		s.Entries[s.LsnLogToBeAdded] = log
	} else { return LogEntry{}, nil }
	s.LsnLogToBeAdded++
	s.LastTerm = term
	return log, nil
}
//~ func (s *SharedLog) Append(term int, data []byte, conn net.Listener) (LogEntry, error) {
	//~ s.Append(term, data)
//~ }

// Adds the command in the shared log to the input_ch
func (s *SharedLog) Commit(sequenceNumber Lsn) {
	lsnToCommit := s.r.LsnToCommit
	// Adds the commands to the input_ch to be furthur processed by Evaluator
	for i:=lsnToCommit; i<Lsn(len(s.Entries)) && i<=sequenceNumber; i++ {
		s.r.Input_ch <- String_Conn{string(s.Entries[i].Command), s.Entries[i].Client}
		s.Entries[i].IsCommitted = true
	}
	s.r.LsnToCommit++
}

// assuming Entries[i] does not have Lsn = i
func (s *SharedLog) GetLog(sequenceNumber Lsn) (*LogEntry) {
	for i:=len(s.Entries)-1; i>=0; i-- {
		if sequenceNumber == s.Entries[i].SequenceNumber {
			return &s.Entries[i];
		}
	}
	l := LogEntry{}
	l.Term = -1
	l.SequenceNumber = -1
	return &l;
}

// ------------- RPC -------------------
type RPC struct {
}
func (a *RPC) AppendRPC(args AppendRequest, reply *AppendResponse) error {
	//~ log.Print("Received", args)
	Server.AppendInput_ch <- args
	//~ log.Print("Replied AppendReply ", reply)
	*reply = <-Server.AppendOutput_ch
	return nil
}
func (a *RPC) CommitRPC(args Lsn, reply *string) error {
	Server.CommitInput_ch <- args
	*reply = <-Server.CommitOutput_ch			// doesn't matter
	//~ log.Print("Replied CommitReply ", reply)
	return nil
}
func (a *RPC) VoteRPC(args VoteRequest, reply *VoteResponse) error {
	Server.VoteInput_ch <- args
	*reply = <-Server.VoteOutput_ch
	return nil
}
