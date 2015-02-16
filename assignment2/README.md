# RAFT

This is the Second assignment of course CS733: Advanced Distributed Computing - Engineering a Cloud. 
The purpose of this assignment is to understand and implement the functionalities of the Leader in RAFT protocol.
We have a set of servers. One of them is made the leader, which communicates with the clients over TCP. 
Leader communicates with the followers through RPC. 	

# Files

This assignment contains 2 files (main & test file) and 2 library files

# Description

Client can send request to any running server. If server is follower it sends `ERR_REDIRECT` message along with host name 
and port number of the leader. With this information the client can make TCP connection with it. If the server is the leader, it accepts the command, checks if it is not a read request and add it to it shared log.
It then updates the shared log of the other servers via RPC call, after which it commits immediately.

The servers can be executed in any order. One will wait for the other to spawn and then connect. The `main()` creates a new `Raft` object initializing it with all the server details.

Following is the description of implemented functions in program

 - `func Init`: It initializes the server, i.e. starts threads for different purposes.
 - `func AcceptConnection`: It accepts an incoming TCP connection request at port 9000 (leader), and spawn handler `ClientListener` for that client.
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
 
 
 
