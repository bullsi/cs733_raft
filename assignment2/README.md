# RAFT

 This is the Second assignment of course CS733: Advanced Distributed Computing - Engineering a Cloud. 
The perpose this assignment is to understand and implement the functionalities of Leader in RAFT protocol.
We have built a set of Severs(n). One of them is made Leader, who communicates with client over TCP. 
Leadaer communicates with follower through RPC. 	

# Files

This assignment contaians 2 files(main & test file) and 2 library files

# Description

 Client can send request to any runnig server. If server is follower it sends ERR_REDIRECT message along with host name 
and port number of Leader server. With this information Client make TCP connection with Leader. Once connection is established
client can send command to Leader,who accept it,processes it and makes entry into his shared log. Now leader send this data to all
follower through RPC to add it in their corresponding shared log. After sending RPC to all follower, leader commits the entry.

Following is the description of implemented functions in programme 

 - `func Init`: It initialises server,i.e. starts four functions AcceptConnection,ClientListener,Evaluator,DataWriter.
 - `func AcceptConnection`: accepts an incoming connection request at port 9000.It spawns three types goroutines to process the command
 - `func ClientListener`: It is used to take input request from a client. Hence, it is spawned for every client whenever a connection is made.
 - `func Evaluator`: It reads from the input channel `input_ch`, processes the request and sends the reply to the output channel `output_ch`.
 - `func DataWriter`: It reads from the output channel `output_ch`, and sends the output to the respective client (if required).
 - `func ConnectToServers`: This function is executed if server leader and connect all other sever.
 - `func AcceptRPC`: This function is executed if server is follower and establishes RPC connection with each other.
 - `func Append`: This appends data in shared log
 - `func Commit`: Makes the data entry into KVStore

# Usage


