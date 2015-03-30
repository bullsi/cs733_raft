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
and ask others for vote sending their `{Id, Term, LastLogLsn, LastLogTerm}` in order for them to decide, whether to give vote or not.
Once a candidate gets a majority of vote, it becomes the leader.
The leader starts sending heartbeats (`AppendRequest` messages with no command) after every `HeartBeatTimer`.

### Log Replication
It accepts commands from the client and adds them to its log.
It then asks others to do so sending them its `{Id, Entry, PrevTerm}`. The follower can then decide whether to accept the log or not.
Once it gets a majority of confirmations that the followers have appended the entry, it commits up to that point i.e. evaluate the command and change the state of the KV Store.
If the follower doesn't accept, that means it is not in sync. Then, the leader keep sending previous logs and search for a sync point.
Once it finds one, the normal procedure continues.

# Implementation

The struct `RaftServer` encapsulates the details of the server.
It contains the data related to it and the components, running as go routines, to process a request.
`func main()` creates its 5 concurrent instances. They communicate using channels unlike RPCs like before.

Following is the description of the implemented functions of `RaftServer`
### Channels for server-server communication
 - `VoteInput_ch`: An instance of `VoteRequest` for asking vote.
 - `VoteOutput_ch`: An instance of `VoteResponse` as the reply of the above.
 - `AppendInput_ch`: An instance of `AppendRequest` asking the server to append an entry.
 - `AppendOutput_ch`: An instance of `AppendResponse` as the reply of the above.
 - `CommitInput_ch`: An instance of `Lsn` asking the server to commit up to an entry.
 - `CommitOutput_ch`: An instance of `strign` as the reply of the above. It is of no use as of now - added for completeness sake.

Client-server communication is done via TCP as in `assignment2`

## Channels for intra-server communication
The leader receives a client request, process it and forms a command.
If that command is not a `get` or `getm` (ie a command which changes the state of the KV store), it undergoes the following sequence.
 - It is added to the `append_ch`.
 - `func AppendCaller()`
   - It takes the command from `append_ch` and ask every other server using `AppendInput_ch` to update their logs.
   - It waits for their response using `AppendOutput_ch` with a timeout of 1 second (hardcoded).
   - If it gets majority of votes, the command is added to `commit_ch`
 - `func CommitCaller()`
   - It takes the command from `commit_ch` and appends it to `input_ch` to execute it on the KV store.
   - It also asks others to commit using `CommitInput_ch`.
 - `func Evaluator()`
   - It takes the command from `input_ch`, executes it and append the output to `output_ch`.
 - `func DataWriter()`
   - It finally takes the command from `output_ch` and send it to the client.
 
Here are some other functions
 - `func AcceptConnection`: It accepts an incoming TCP connection request, and spawn handler `ClientListener` for that client.
 - `func Init`: It initializes the server, i.e. define variables and channels and starts threads for different purposes.
 - `func ClientListener`: It is used to take input request from a client.
 
Meanwhile, the leader also sends heartbeats after every `HeartBeatTimer`. This is nothing but an empty Log.

# Usage

In our case, running the server is somewhat a manual process.
 - Go to the `assignment3` directory
 - Set the environment variable `GOPATH` as the path to the `assignment3` directory.
 - In `program.go`, set the variable `N` to be the number of servers you are going to spawn.
 - Now spawn the servers concurrently using `go run program.go`.
 - Type `go test` to run the test cases.
 
