package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type void struct {}

type waitParam struct {
	toa int64
	c chan int64
}

var (
	flock sync.Mutex
	lock sync.Mutex
	portList map[int]void
	m void
)
//frames : 1496
//JumboFrames : 8996
const BufferSize = 1496

func getPort() int {
	lock.Lock()
	defer lock.Unlock()
	for k := range portList{
		delete(portList, k)
		return k
	}
	fmt.Println("Plus de ports disponibles")
	return -1
}

func releasePort(port int) {
	if port > 0 {
		lock.Lock()
		defer lock.Unlock()
		portList[port] = m
	}
}

func testPort(p int) int {
	port := strconv.Itoa(p)
	pc, err := net.ListenPacket("udp", "0.0.0.0:"+port)
	if err == nil {
		pc.Close()
		return p
	}else{
		fmt.Println("erreur avec le port " + port)
		return -1
	}
}

func testPorts(portMin int, portMax int) {
	for p:=portMin; p<=portMax; p++ {
		releasePort(testPort(p))
	}
}

// https://kgrz.io/reading-files-in-go-an-overview.html
func readFile(file string) ([][]byte, int){
	//file, _ = regexp.MatchString()
	//file = "coucou "
	flock.Lock()
	defer flock.Unlock()
	absFile, _ := filepath.Abs(file)
	f, err := os.Open(absFile)
	if err != nil{
		fmt.Println("Error while openning", absFile)
		return nil, 0
	}
	defer f.Close()
	data := make([][]byte, 0)
	n := 0
	m := 0
	for {
		d := make([]byte, BufferSize)
		n, _ = f.Read(d)
		//fmt.Println("Nombre de bytes lus", n)
		if n == 0{
			break
		}
		m = n
		if err != nil {
			if err != io.EOF {
				fmt.Println(err)
				return nil, 0
			}
			data = append(data, d)
			break
		}
		data = append(data, d)
	}
	return data, m
}

func readpc(pc net.PacketConn, ch chan string){
	for{
		buffer := make([]byte, 100)
		n,_,_ := pc.ReadFrom(buffer)
		ch <- string(buffer[:n])
	}
}

func sendFile(file string, pc net.PacketConn, add net.Addr, rtt int64) bool {
	data, last_len := readFile(file)
	bytes, seqn0 := 0, 000001
	ch := make(chan string)
	go readpc(pc, ch)
	for i := 0; i < len(data); i++ {
		//toSend := make([]byte, 1500)
		bs := fmt.Sprintf("%06d", seqn0)
		for {
			if (i == len(data)-1){
				fmt.Println("last len", last_len)
				data[i] = data[i][:last_len]
			}
			toSend := append([]byte(bs), data[i]...)
			pc.WriteTo(toSend, add)
			sbuffer := ""
			select{
			case <- time.After(time.Duration(rtt+50000)*time.Nanosecond):
					sbuffer = "erreur"
				case sbuffer = <- ch:
			}
			if(strings.Contains(sbuffer, bs)){
				break
			}
		}
		bytes += len(data[i])
		seqn0 ++
	}
	pc.WriteTo([]byte("FIN"), add)
	fmt.Println("Nombre de bytes lus :", bytes)
	return true
}

func handleClient(add net.Addr, port int, c chan int64){
	defer releasePort(port)
	defer fmt.Println("FIN Transmission")
	pc, err := net.ListenPacket("udp", "0.0.0.0:"+strconv.Itoa(port))
	if err != nil {
		log.Fatal(err)
	}
	defer pc.Close()
	var rtt int64
	select {
	case rtt =<-c:
		fmt.Println("ok - RTT", rtt, "ns (1ms = 1 000 000ns)")
		if rtt == 0 {
			fmt.Println("Deleted")
			return
		}
	case <- time.After(10 * time.Second):
		fmt.Println("Deleted")
		return
	}
	for {
		buffer := make([]byte, 1024)
		n, _, _ := pc.ReadFrom(buffer)
		fmt.Println("handle", port, add,"\n"+string(buffer[:n]), n)
		start := time.Now()
		var t time.Time
		if sendFile(string(buffer[:n-1]), pc, add, rtt){
			t = time.Now()
			fmt.Println("file ", string(buffer), "\n Duration of transfer : ", t.Sub(start))
			break
		}
	}
}

func main(){
	args := os.Args[1:]
	port := "8080"
	addrWait := make(map[string]waitParam)

	if len(args) == 1 {
		port = args[0]
	}

	fmt.Println("Testing ports")
	portList = make(map[int]void)
	testPorts(1000,9999)
	fmt.Println("Initial portList has been set")

	fmt.Println("Launching servern, on port", port)

	pc, err := net.ListenPacket("udp", "0.0.0.0:"+port)
	if err != nil {
		log.Fatal(err)
	}
	defer pc.Close()

	for {
		fmt.Println("ACK waiting :", len(addrWait))
		buffer := make([]byte, 1024)
		_, addr, err := pc.ReadFrom(buffer)
		if err != nil {
			log.Fatal(err)
		}
		in := string(buffer)
		add := addr.String()
		fmt.Println(in ,addr)
		if strings.Contains(in, "SYN") {
			for k, v := range addrWait{
				fmt.Println(k,time.Now().UnixNano()-  v.toa )
				if time.Now().UnixNano() - v.toa > 1000000000 {
					fmt.Println("Deleting", k)
					close(v.c)
					delete(addrWait, k)
				}
			}
			p := getPort()
			if testPort(p) > 0 {
				ch := make(chan int64)
				go handleClient(addr, p, ch)
				s := waitParam{
					toa: time.Now().UnixNano(),
					c:   ch,
				}
				addrWait[add] = s
				pc.WriteTo([]byte("SYN-ACK"+strconv.Itoa(p)), addr)
			}

		}else if el, found := addrWait[add]; strings.Contains(in, "ACK") && found{
			fmt.Println("Got ACK")
			el.c <- time.Now().UnixNano() - el.toa //RTT dans la goroutine
			close(el.c)
			delete(addrWait, add)
		}else{
			fmt.Println(addrWait)
			continue
		}
	}
}
