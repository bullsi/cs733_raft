package assignment1

import (
//	"fmt"
	"log"
	"time"
	"strings"
	"net"
	"strconv"
)

type Value struct {
	text			string
	expiryTime		time.Time
	isExpTimeInf	bool
	numBytes		int
	version			int64
}

type String_Conn struct {
	text string
	conn net.Conn
}

var values = make(map[string]Value)

var input_ch = make(chan String_Conn, 10000)
var output_ch = make(chan String_Conn, 10000)

// Reads send-requests from the output channel and processes it
func DataWriter() {
	for {
		replych := <-output_ch
		text := replych.text
		conn := replych.conn
		if text != "" {
			conn.Write([]byte(text))
		}
	}
}

// Executes the set command
func Set(command string, conn net.Conn) {
	inputs_ := strings.Split(command, "\r\n")
	inputs := strings.Split(inputs_[0], " ")
	key := inputs[1]
	text := inputs_[1]
	seconds, err := strconv.Atoi(inputs[2])
	if err != nil {
		output_ch <- String_Conn{"ERR_CMD_ERR\r\n", conn}
		return
	}
	num_bytes, err := strconv.Atoi(inputs[3]) 
	if num_bytes != len(text) || err != nil {
		output_ch <- String_Conn{"ERR_CMD_ERR\r\n", conn}
		return
	}

	t, present := values[key]

	// value not present or present but expired
	if !present || (present && !t.isExpTimeInf && t.expiryTime.Before(time.Now())){
		values[key] = Value{
			text: text,
			expiryTime: time.Now().Add(time.Duration(seconds) * time.Second),
			isExpTimeInf: (seconds == 0),
			numBytes: num_bytes,
			version: 1,
		}
	} else {
		values[key] = Value{
			text: text,
			expiryTime: time.Now().Add(time.Duration(seconds) * time.Second),
			isExpTimeInf: (seconds == 0),
			numBytes: num_bytes,
			version: t.version + 1,
		}
	}
	if len(inputs) == 5 && inputs[4] == "noreply" {
		return
	} else {
		output_ch <- String_Conn{"OK " + strconv.FormatInt(values[key].version,10) + "\r\n", conn}
		return
	}
}

// Executes the get command
func Get(command string, conn net.Conn) {
	inputs_ := strings.Split(command, "\r\n")
	inputs := strings.Split(inputs_[0], " ")
	key := inputs[1]
	t, present := values[key]
	if !present || (present && !t.isExpTimeInf && t.expiryTime.Before(time.Now())) {
		if present { delete(values, key) }
		output_ch <- String_Conn{"ERR_NOT_FOUND\r\n", conn}
		return
	} else {
		output_ch <- String_Conn{"VALUE " + strconv.Itoa(t.numBytes) + "\r\n" + t.text + "\r\n", conn}
		return
	}
}

// Executes the getm command
func Getm(command string, conn net.Conn) {
	inputs_ := strings.Split(command, "\r\n")
	inputs := strings.Split(inputs_[0], " ")
	key := inputs[1]
	t, present := values[key]

	if !present || (present && !t.isExpTimeInf && t.expiryTime.Before(time.Now())){
		if present { delete(values, key) }
		output_ch <- String_Conn{"ERR_NOT_FOUND\r\n", conn}
		return
	} else {
		secondsLeft := strconv.Itoa(int(t.expiryTime.Sub(time.Now()).Seconds()))
		if t.isExpTimeInf {
			secondsLeft = "0"
		}
		output_ch <- String_Conn{"VALUE " + strconv.Itoa(int(t.version)) + " " + secondsLeft + " " + strconv.Itoa(t.numBytes) + "\r\n" + t.text + "\r\n", conn}
		return
	}
}

// Executes the cas command
func Cas(command string, conn net.Conn) {
	inputs_ := strings.Split(command, "\r\n")
	inputs := strings.Split(inputs_[0], " ")
	key := inputs[1]
	seconds, err := strconv.Atoi(inputs[2])
	if err != nil {
//		log.Println("AA1")
		output_ch <- String_Conn{"ERR_CMD_ERR\r\n", conn}
		return
	}
	version, err := strconv.ParseInt(inputs[3],10,64)
	if err != nil {
//		log.Println("AA2")
		output_ch <- String_Conn{"ERR_CMD_ERR\r\n", conn}
		return
	}
	num_bytes, err := strconv.Atoi(inputs[4])
	if err != nil {
//		log.Println("AA3")
		output_ch <- String_Conn{"ERR_CMD_ERR\r\n", conn}
		return
	}
	
//	log.Println("sss")

	text := inputs_[1]
	t, present := values[key]

	if !present || (present && !t.isExpTimeInf && t.expiryTime.Before(time.Now())){
		if present { delete(values, key) }
		output_ch <- String_Conn{"ERR_NOT_FOUND\r\n", conn}
		return
	} else {
		if t.version == version {
			values[key] = Value {
				text: text,
				expiryTime: time.Now().Add(time.Duration(seconds) * time.Second),
				isExpTimeInf: (seconds == 0),
				numBytes: num_bytes,
				version: t.version+1,
			}
		}
	}
	if len(inputs) == 6 && inputs[5] == "noreply" {
		return
	} else {
		if t.version != version {
			output_ch <- String_Conn{"ERR_VERSION\r\n", conn}
			return
		} else {
			output_ch <- String_Conn{"OK " + strconv.FormatInt(values[key].version,10) + "\r\n", conn}
			return
		}
	}
}

// Executes the delete command
func Delete(command string, conn net.Conn) {
	inputs_ := strings.Split(command, "\r\n")
	inputs := strings.Split(inputs_[0], " ")
	key := inputs[1]
	t, present := values[key]
	if !present || (present && !t.isExpTimeInf && t.expiryTime.Before(time.Now())){
		if present { delete(values, key) }
		output_ch <- String_Conn{"ERR_NOT_FOUND\r\n", conn}
		return
	} else {
		delete(values, key)
		output_ch <- String_Conn{"DELETED\r\n", conn}
		return
	}
}


// Reads instruction from the input channel, processes it and adds the reply to the output queue
func Evaluator() {
	for {
	
		inputch := <-input_ch			// Only one control flow is allowed after this point
		input := inputch.text
		conn := inputch.conn
		
		inputs_ := strings.Split(input, "\r\n")
		inputs := strings.Split(inputs_[0], " ")

		if inputs[0] == "set" {
			Set(input, conn)
		}
		if inputs[0] == "get" {
			Get(input, conn)
		}
		if inputs[0] == "getm" {
			Getm(input, conn)
		}
		if inputs[0] == "cas" {
			Cas(input, conn)
		}
		if inputs[0] == "delete" {
			Delete(input, conn)
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

// ClientListener is spawned for every client to retrieve the command and send it to the input_ch channel
func ClientListener(listener net.Conn) {
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

		command, rem = GetCommand(rem + input_)

		// For multiple commands in the byte stream
		for {
			// Process and Send
			if command != "" {
				input_ch <- String_Conn{command,listener}
			} else { break }
			command, rem = GetCommand(rem)
		}
	}
}

// Accepts an incoming connection request, and spawn a ClientListener
func AcceptConnection() {
	go Evaluator()
	go DataWriter()
	
	tcpAddr, error := net.ResolveTCPAddr("tcp", "localhost:9000")

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
		go ClientListener(listener)
	}
}

/*func main() {
	go AcceptConnection()
}*/
