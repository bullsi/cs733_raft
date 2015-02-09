package raft

import(
	"strings"
	"time"
	"net"
	"strconv"
//	"log"
)
// Executes the set command
func Set(command string, conn net.Conn) {
	inputs_ := strings.Split(command, "\r\n")
	inputs := strings.Split(inputs_[0], " ")

	key := inputs[1]
	text := inputs_[1]
	seconds, err := strconv.Atoi(inputs[2])
	if err != nil {
		Output_ch <- String_Conn{"ERR_CMD_ERR\r\n", conn}
		return
	}
	num_bytes, err := strconv.Atoi(inputs[3]) 
	if num_bytes != len(text) || err != nil {
		Output_ch <- String_Conn{"ERR_CMD_ERR\r\n", conn}
		return
	}

	t, present := KVStore[key]

	// value not present or present but expired
	if !present || (present && !t.IsExpTimeInf && t.ExpiryTime.Before(time.Now())) {
		KVStore[key] = Value{
			Text: text,
			ExpiryTime: time.Now().Add(time.Duration(seconds) * time.Second),
			IsExpTimeInf: (seconds == 0),
			NumBytes: num_bytes,
			Version: 1,
		}
	} else {
		KVStore[key] = Value{
			Text: text,
			ExpiryTime: time.Now().Add(time.Duration(seconds) * time.Second),
			IsExpTimeInf: (seconds == 0),
			NumBytes: num_bytes,
			Version: t.Version + 1,
		}
	}
	if len(inputs) == 5 && inputs[4] == "noreply" {
		return
	} else {
		Output_ch <- String_Conn{"OK " + strconv.FormatInt(KVStore[key].Version,10) + "\r\n", conn}
		return
	}
}
func Get(command string, conn net.Conn) {
	inputs_ := strings.Split(command, "\r\n")
	inputs := strings.Split(inputs_[0], " ")
	key := inputs[1]
	t, present := KVStore[key]
	if !present || (present && !t.IsExpTimeInf && t.ExpiryTime.Before(time.Now())) {
		if present { delete(KVStore, key) }
		Output_ch <- String_Conn{"ERR_NOT_FOUND\r\n", conn}
		return
	} else {
		Output_ch <- String_Conn{"VALUE " + strconv.Itoa(t.NumBytes) + "\r\n" + t.Text + "\r\n", conn}
		return
	}
}

// Executes the getm command
func Getm(command string, conn net.Conn) {
	inputs_ := strings.Split(command, "\r\n")
	inputs := strings.Split(inputs_[0], " ")
	key := inputs[1]
	t, present := KVStore[key]

	if !present || (present && !t.IsExpTimeInf && t.ExpiryTime.Before(time.Now())){
		if present { delete(KVStore, key) }
		Output_ch <- String_Conn{"ERR_NOT_FOUND\r\n", conn}
		return
	} else {
		secondsLeft := strconv.Itoa(int(t.ExpiryTime.Sub(time.Now()).Seconds()))
		if t.IsExpTimeInf {
			secondsLeft = "0"
		}
		Output_ch <- String_Conn{"VALUE " + strconv.Itoa(int(t.Version)) + " " + secondsLeft + " " + strconv.Itoa(t.NumBytes) + "\r\n" + t.Text + "\r\n", conn}
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
		Output_ch <- String_Conn{"ERR_CMD_ERR\r\n", conn}
		return
	}
	version, err := strconv.ParseInt(inputs[3],10,64)
	if err != nil {
		Output_ch <- String_Conn{"ERR_CMD_ERR\r\n", conn}
		return
	}
	num_bytes, err := strconv.Atoi(inputs[4])
	if err != nil {
		Output_ch <- String_Conn{"ERR_CMD_ERR\r\n", conn}
		return
	}

	text := inputs_[1]
	t, present := KVStore[key]

	if !present || (present && !t.IsExpTimeInf && t.ExpiryTime.Before(time.Now())){
		if present { delete(KVStore, key) }
		Output_ch <- String_Conn{"ERR_NOT_FOUND\r\n", conn}
		return
	} else {
		if t.Version == version {
			KVStore[key] = Value {
				Text: text,
				ExpiryTime: time.Now().Add(time.Duration(seconds) * time.Second),
				IsExpTimeInf: (seconds == 0),
				NumBytes: num_bytes,
				Version: t.Version+1,
			}
		}
	}
	if len(inputs) == 6 && inputs[5] == "noreply" {
		return
	} else {
		if t.Version != version {
			Output_ch <- String_Conn{"ERR_VERSION\r\n", conn}
			return
		} else {
			Output_ch <- String_Conn{"OK " + strconv.FormatInt(KVStore[key].Version,10) + "\r\n", conn}
			return
		}
	}
}

// Executes the delete command
func Delete(command string, conn net.Conn) {
	inputs_ := strings.Split(command, "\r\n")
	inputs := strings.Split(inputs_[0], " ")
	key := inputs[1]
	t, present := KVStore[key]
	if !present || (present && !t.IsExpTimeInf && t.ExpiryTime.Before(time.Now())){
		if present { delete(KVStore, key) }
		Output_ch <- String_Conn{"ERR_NOT_FOUND\r\n", conn}
		return
	} else {
		delete(KVStore, key)
		Output_ch <- String_Conn{"DELETED\r\n", conn}
		return
	}
}
// Reads send-requests from the output channel and processes it

func DataWriter() {
	for {
		replych := <-Output_ch
		text := replych.Text
		conn := replych.Conn		// nil for followers as they don't have to send the reply back
		if text != "" && conn != nil {
			conn.Write([]byte(text))
		}
	}
}

// Reads instruction from the input channel, processes it and adds the reply to the output queue
func Evaluator() {
	for {
		inputch := <-Input_ch			// Only one control flow is allowed after this point
		
		input := inputch.Text
		conn := inputch.Conn
		
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

