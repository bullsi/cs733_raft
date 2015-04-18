package main

import (
	"raft"
	"fmt"
	//~ "log"
	//~ "net/rpc"
	"os"
	"strconv"
)

var N int = 5			// Number of servers


// Creates a raft object. This implements the SharedLog interface.
// commitCh is the channel that the kvstore waits on for committed messages.
// When the process starts, the local disk log is read and all committed
// entries are recovered and replayed
func NewRaft(totalServers int, config *raft.ClusterConfig, thisServerId int) (*raft.RaftServer, error) {
	raft := new(raft.RaftServer)
	raft.Init(totalServers, config, thisServerId)
	return raft, nil
}

// --------------------------------------------------------------
func main() {
	raft.AllServers = make([]*raft.RaftServer, N)
	id, _ := strconv.Atoi(os.Args[1])
	sConfigs := make([]raft.ServerConfig, N)
	for i:=0; i<N; i++ {
		sConfigs[i] = raft.ServerConfig{
			Id: i,
			Hostname: "Server"+strconv.Itoa(i),
			ClientPort: 9000+2*i, 
			LogPort: 9001+2*i,
			Client: nil,
			//~ LsnToCommit: 0,
		}
	}
	cConfig := raft.ClusterConfig{"undefined path", -1, sConfigs}		// -1 for no leader
	raft.Server, _ = NewRaft(N, &cConfig, id)

	// Wait until some key is press
	var s string
	fmt.Scanln(&s)
}

