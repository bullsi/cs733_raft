# Introduction

This is the Third assignment of course CS733: Advanced Distributed Computing - Engineering a Cloud. 
It is basically an extension of the previous assignment. In this, I have implemented the process of Leader Election and Log
Replication. Earlier each server was a different process but now it is a separate go routine. Moreover, server-server interation was
using RPC but now it is through channels for simplicity. The focus is not on the I/O but on the correctness of the implementation.

# Files

This assignment contains a main, test and 2 library files

# Brief Description

Client can send request to any running server. If server is a follower it sends `ERR_REDIRECT` message along with host name 
and port number of the leader. With this information the client can make TCP connection with it.
If the server is the leader, it accepts the command, checks if it is not a read request and add it to it shared log.
It then updates the shared log of the other servers via RPC call, after which it commits immediately.

### Leader Election
Unlike before, when server-0 was the leader, all the servers start in the follower state with an `ElectionTimer`.
The followers whose timer expire become the candidates. They first vote for themselves
and ask others for vote sending their {Id, Term, LastLogLsn, LastLogTerm} in order for them to decide, whether to give vote or not.
Once a candidate gets a majority of vote, it becomes the leader.
The leader starts sending heartbeats (`AppendRequest` messages with no command) after every `HeartbeatTimer`.

### Log Replication
It accepts commands from the client and adds them to its log.
It then asks others to do so sending them its {Id, Entry, PrevTerm}. The follower can then decide whether to accept the log or not.
Once it gets a majority of confirmations that the followers have appended the entry, it commits up to that point i.e. evaluate the command and change the state of the KV Store.
If the follower doesn't accept, that means it is not in sync. Then, the leader keep sending previous logs and search for a sync point.
Once it finds one, the normal procedure continues.

# Implementation

The struct `RaftServer` encapsulates the details of the server.
The servers can be executed in any order. One will wait for the other to spawn and then connect.
The `main()` creates a new `Raft` object initializing it with all the server details.

Following is the description of implemented functions in program

 - `func Init`: It initializes the server, i.e. starts threads for different purposes.
 - `func AcceptConnection`: It accepts an incoming TCP connection request, and spawn handler `ClientListener` for that client.
 - `func ClientListener`: It is used to take input request from a client.
 - `func Evaluator`: It reads from the input channel `input_ch`, processes the request and sends the reply to the output channel `output_ch`.
 - `func DataWriter`: It reads from the output channel `output_ch`, and sends the output to the respective client (if required).
 - `func ConnectToServers`: It is executed to connect each server to every other server.
 - `func AcceptRPC`: Similar to `AcceptConnection`, this function accepts RPC connection request at port 9001 (leader).
 - `func Append`: This appends data in shared log.
 - `func Commit`: Runs the command and commits to the KVStore.

# Usage

In our case, running the server is somewhat a manual process.
 - Go to the `assignment2` directory
 - Set the environment variable `GOPATH` as the path to the `assignment2` directory.
 - In `program.go`, set the variable `N` to be the number of servers you are going to spawn.
 - Now spawn the servers as `go run program.go <id>`. The server with `id = 0` is the leader. Hostname and port numbers of the servers are decided based on this.
 <br/> We tried to automate this process but the program didn't work.
 - Type `go test` to run the test cases.
 
 
 
