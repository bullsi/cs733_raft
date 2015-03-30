package main

import "fmt"
import "os"
import "os/exec"
import "strconv"
import "sync"

var wg sync.WaitGroup

func StartServer(id int) {
	cmd := exec.Command("./program", strconv.Itoa(id))
	output, err := cmd.Output()
	if err != nil {
		fmt.Println("Error:",err)
	}
	fmt.Println(string(output))
}

func main() {

	fmt.Print("---")
	wg.Add(1)
	go func(){
		cmd := exec.Command("../program", "0")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		if err != nil { panic(err) }
		defer wg.Done()
	}()
	
	wg.Add(1)
	go func(){
		cmd := exec.Command("../program", "1")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		if err != nil { panic(err) }
		defer wg.Done()
	}()
	
	wg.Add(1)
	go func(){
		cmd := exec.Command("../program", "2")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		if err != nil { panic(err) }
		defer wg.Done()
	}()
	
	wg.Add(1)
	go func(){
		cmd := exec.Command("../program", "3")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		if err != nil { panic(err) }
		defer wg.Done()
	}()
	
	wg.Add(1)
	go func(){
		cmd := exec.Command("../program", "4")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		if err != nil { panic(err) }
		defer wg.Done()
	}()
	var str string
	fmt.Scanln(&str)
}
