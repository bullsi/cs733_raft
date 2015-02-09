# Memcached Clone
This is our first assignment of the course CS733: Advanced Distributed Computing - Engineering a Cloud.
I have build a single server that implements a versioned map (explained later).
The purpose of the assigment is to be able to handle concurrent requests consistently.

# Files
The assignment consists of two files

### program.go
It implements the memcached clone server.
`func AcceptConnection` accepts an incoming connection request at port 9000.
It spawns three types goroutines to process the command
 - `func ClientListener`: It is used to take input request from a client. Hence, it is spawned for every client whenever a connection is made.
 - `func Evaluator`: It reads from the input channel `input_ch`, processes the request and sends the reply to the output channel `output_ch`.
 - `func DataWriter`: It reads from the output channel `output_ch`, and sends the output to the respective client (if required).

Every client is assigned a separate go routine `func ClientListener` which reads a command from the client, adds it to `input_ch` channel for processing. The server works even if the command is broken into packets or one packet comprises of many commands.

### program\_test.go
This program implements the test cases.
It first launches the server in `func TestMain` and then, in `func TestCases1`, spawns `n` concurrent clients using go routines.
A channel is maintained to check if all the clients have finished execution.


# Usage
To run the program, go to the `assignment1` folder and type `go test` in the terminal.

# Protocol Specifications
 - Set: create the key-value pair, or update the value if it already exists.
```
set <key> <exptime> <numbytes> [noreply]\r\n
<value bytes>\r\n
```

    The server responds with:
```
    OK <version>\r\n  
```
    where version is a unique 64-bit number (in decimal format) assosciated with the key.
 -  Get: Given a key, retrieve the corresponding key-value pair
```
    get <key>\r\n
```
    The server responds with the following format (or one of the errors described later)
```
    VALUE <numbytes>\r\n
    <value bytes>\r\n
```
 -  Get Meta: Retrieve value, version number and expiry time left
```
     getm <key>\r\n
```
    The server responds with the following format (or one of the errors described below)
```
    VALUE <version> <exptime> <numbytes>\r\n
    <value bytes>\r\n
```
 -  Compare and swap. This replaces the old value (corresponding to key) with the new value only if the version is still the same.
```
    cas <key> <exptime> <version> <numbytes> [noreply]\r\n
    <value bytes>\r\n
```
    The server responds with the new version if successful (or one of the errors described late)
```
      OK <version>\r\n
```
 -  Delete key-value pair
```
     delete <key>\r\n
```
    Server response (if successful)
```
      DELETED\r\n
```

### Things to be considered
 - `listener.SetDeadline(time.Now().Add(3 * time.Second))` puts us the timer. I'm not sure it it closes the connection, but it stops accepting any more inputs from the client.
 - `go test -race` is not supported in my system.



