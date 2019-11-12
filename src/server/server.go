package main

import ("fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type void struct {}

var (
	lock sync.Mutex
	portList map[int]void
	m void
)

func getPort() int {
	lock.Lock()
	defer lock.Unlock()
	for k := range portList{
		delete(portList, k)
		return k
	}
	fmt.Println("Plus de ports disponibles")
	return 0
}

func releasePort(port int) {
	lock.Lock()
	defer lock.Unlock()
	portList[port] = m
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

func handleClient(add net.Addr, port int){
	defer releasePort(port)
	pc, err := net.ListenPacket("udp", "0.0.0.0:"+strconv.Itoa(port))
	if err != nil {
		log.Fatal(err)
	}
	defer pc.Close()
	for ; ; {
		buffer := make([]byte, 1024)
		_, _, err = pc.ReadFrom(buffer)
		fmt.Println("handle", port, add, "\n", string(buffer))
		fmt.Println(buffer)
		if strings.Contains(string(buffer), "FIN"){
			return
		}
	}

}

func main(){
	args := os.Args[1:]
	port := "8080"
	addrWait := make(map[string]int64)

	if len(args) == 1 {
		port = args[0]
	}

	fmt.Println("Testing ports")
	portList = make(map[int]void)
	testPorts(1000,9999)
	fmt.Println("Initial portList has been set")

	fmt.Println("Launching server")

	pc, err := net.ListenPacket("udp", "0.0.0.0:"+port)
	if err != nil {
		log.Fatal(err)
	}
	defer pc.Close()

	for ; ; {
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
			p := getPort()
			if testPort(p) > 0 {
				go handleClient(addr, p)
			}
			t := time.Now()
			addrWait[add] = t.UnixNano()
			pc.WriteTo([]byte("SYN-ACK"+strconv.Itoa(p)), addr)
		}else if _, ok := addrWait[add]; strings.Contains(in, "ACK") && ok{
			//check RTT
			delete(addrWait, add)
		}else{
			continue
		}
	}
}