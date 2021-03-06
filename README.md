# Introduction

This (`./assignment4`) is the complete assignment of course CS733: Advanced Distributed Computing - Engineering a Cloud. 
The purpose of this assignment is to understand and implement the functionalities of [RAFT consensus algorithm](https://raft.github.io/). I have implemented the process of Leader Election and Log Replication. In the previous versions of the assignment, each server was a different go routine but now it is again separate go process. Moreover, server-server interation was using channels but now an additional layer of RPC is added over that. The focus is not on the I/O but on the correctness of the implementation.

# Files

This assignment contains a main, test and 2 library files

# Brief Description

Client can send request to any running server. If the server is a follower, it sends `ERR_REDIRECT` message along with the host name 
and port number of the leader. With this information the client can make TCP connection with it.
If the server is the leader, it accepts the command, checks if it is not a read request and add it to it shared log.
It then updates the shared log of the other servers via RPC call, after which it commits immediately.

### Leader Election
Unlike before, when server-0 was the leader, all the servers start in the follower state with an `ElectionTimer`.
The followers whose timer expire become the candidates. They first vote for themselves
and ask others for vote sending their `{Id, Term, LastLogLsn, LastLogTerm}` in order for them to decide, whether to give vote or not.
Once a candidate gets a majority of vote, it becomes the leader.
The leader starts sending heartbeats (`AppendRequest` messages with no command) after every `HeartBeatTimer`.

### Log Replication
It accepts commands from the client and adds them to its log. It then triggers that there is a need to replicate a log using `Append_ch`.
The leader maintains the `NextIndex` of the log to be sent for each follower. It then send them their respective logs in the format `{Id, LogEntry, PrevTerm}`. The follower can then decide whether to accept it or not.
If the leader receives a positive ack, it checks if that log i.e. evaluate the command and change the state of the KV Store.
If the follower doesn't accept, that means it is not in sync. Then, the leader keep sending previous logs and search for a sync point.
Once it finds one, the normal procedure continues.

# Implementation

The struct `RaftServer` encapsulates the details of the server.
It contains the data related to it and the components, running as go routines, to process a request.
`func main()` creates its 5 concurrent instances. They communicate using channels unlike RPCs like before.

Following is the description of the implemented functions of `RaftServer`
## RPC for server-server communication
When a server receives a RPC request, it pushes the input to the input channel. The event corresponding to that is triggered and the output is pushed to the output channel, from where it is replied back.

 - `VoteRPC`: Used to ask for vote in `VoteRequest` format and receive response in `VoteResponse` format. The underlying channels are `VoteInput_ch` and `VoteOutput_ch`.

 - `AppendRPC`: Used to send append requests to follower in `AppendRequest` format and receive response from them in `AppendResponse` format. The underlying channels are `AppendInput_ch` and `AppendOutput_ch`.

 - `CommitRPC`: Used to tell the followers about the committed entries. It sends `Lsn` asking the server to commit up to that entry. The output is of type `string` as the reply. It is of no use - added for completeness sake.

Client-server communication is done via TCP as in `assignment2`

## Channels for intra-server communication
The leader receives a client request, processes it and forms a command.
If that command is not a `get` or `getm` (i.e. a command which changes the state of the KV store), it undergoes the following sequence.

 - It is added to the to the leader's log and `append_ch` is triggered.
 - `func AppendCaller()`
   - Event corresponding to `append_ch` takes place. This tells that there is a need to send some log entries to the follower. It asks every other server using `AppendRPC` call to update their logs.
   - It waits for their response with a timeout of 1 second (hardcoded).
   - If it gets majority of votes, the sequence number is added to `commit_ch`
 - `func CommitCaller()`
   - It takes the sequence number from `commit_ch`. This tells it to commit all the entries up to that. Those entries are pushed into `input_ch` to be executed on the KV store, and marked to be committed.
   - It also asks others to commit using `CommitRPC`.
 - `func Evaluator()`
   - It takes the command from `input_ch`, executes it and pushes the output to `output_ch`.
 - `func DataWriter()`
   - It finally takes the command from `output_ch` and send it to the client.
 
Here are some other functions

 - `func AcceptConnection()`: It accepts an incoming TCP connection request from the client, and spawn handler `ClientListener` for that.
 - `func AcceptRPC()`: It accepts incoming RPC request from other servers.
 - `func Init()`: It initializes the server, i.e. define variables and channels and starts threads for different purposes.
 - `func ClientListener()`: It is used to take input request from a client.
 
# Usage

In our case, running the server is somewhat a manual process.

 - Go to the `assignment4` directory
 - Set the environment variable `GOPATH` as the path to the `assignment4` directory.
 - In `program.go`, set the variable `N` to be the number of servers you are going to spawn.
 - Now spawn the servers concurrently using `go run program.go <id>` for different `<id>`. Spawing servers with `<id>` from 0 to `N-1` is better for using the `program_test.go` file correctly.
 - Type `go test` to run the test cases.
 
