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
	"math"
)

type void struct {}

type ack struct{
	n string
	toa int64
}

type waitParam struct {
	toa int64
	c chan int64
}

type ack_list struct {
	index int
	value string
}


var (
	lock sync.Mutex
	portList map[int]void
	m void
)

const BufferSize = 1494

const attenuation_coefficient float32 = 0.5
const incrementation_ca = 1

// Potential RTT/RTO/SRTT evolution
const R = 0.0
// real initization will be done after first handshake (SYN-ACK -> ACK) will be the R variable (in ms)

const alpha = 1/8 // (RFC6298)
const beta = 1/4 // (RFC6298)
const granularity = 1// granularity of the clock we'll use (think that time library works in ms)
const K = 4 // (RFC6298)

type conn_param struct {
	SRTT float64
	RTTVAR float64
	RTO float64
	cwnd int
	congestion_type string
}

func NewConn_param (r float64) conn_param {
	cp := conn_param{
		SRTT: r,
		RTTVAR: r/2,
		RTO: r + math.Max(granularity, K*(r/2)),
		cwnd: 1,
		congestion_type: "SS",
	}
	return cp
}


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

func update_time_mesure(new_measure float64, cp conn_param) {
	cp.RTTVAR = (1-beta)*cp.RTTVAR + beta*math.Abs(cp.SRTT-new_measure)
	cp.SRTT = (1-alpha)*cp.SRTT + alpha*new_measure
	cp.RTO = cp.SRTT + math.Max(granularity, K*cp.RTTVAR)
}


func cwnd_evolution (flag int, seq_failed int, cp conn_param){
	/*
		Function that will deal with the evolution of our congestion window and
		that will handle the switch from slow start to congestion avoidance and
		so recalculate our new cwnd

		flag : indicates whether there was an error (ACK not received) in a RTO

		---- 0 => everything received
		---- 1 => error

		AIMD implementation
	*/
	fmt.Println("Evolution of cwnd")
	switch flag {
		case 0:
			switch cp.congestion_type{
				case "SS":
					cp.cwnd *= 2
				case "CA":
					cp.cwnd += incrementation_ca
			}
		case 1:
			switch cp.congestion_type{
					case "SS":
						if (seq_failed > 0){
								cp.cwnd = int(float32(cp.cwnd)*attenuation_coefficient)+1 //index ?
								cp.congestion_type="CA"
							}
					case "CA":
						cp.cwnd = int(float32(cp.cwnd)*attenuation_coefficient)+1

					default:
						cp.RTO*=2
						fmt.Println("Congestion => increase RTO (by 2)")
				}

	}
}

// https://kgrz.io/reading-files-in-go-an-overview.html
func readFile(file string) ([][]byte, int){
	//file, _ = regexp.MatchString()
	//file = "coucou "
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

func readpc(pc net.PacketConn, ch chan ack){
	for{
		buffer := make([]byte, 100)
		n,_,_ := pc.ReadFrom(buffer)
		if (n > 0){
		ch <- ack{string(buffer[:n-1]),time.Now().UnixNano()}
	}
	}
}

/*func remove(slice []int, s int) []int {
    return append(slice[:s], slice[s+1:]...)
}
*/

func contains_find(a []ack_list, x string) (bool,int) {
				for i, n := range a {
          if strings.Compare(x,n.value) == 0 {
              return true,i
          }
        }
        return false,0
}

func sendFile(file string, pc net.PacketConn, add net.Addr, cp conn_param) bool {
	data, last_len := readFile(file) // data : array of data size of buffer
	fmt.Println("Data longeur ",len(data))
	seqn0, i := 000001, 0
	ch := make(chan ack, 1000)
	go readpc(pc, ch)
	var ack_array []ack_list // Initial array with all expected ACKs

	for i < len(data){
		real_size := cp.cwnd - len(ack_array)
		fmt.Println("Taille de la fenêtre ", cp.cwnd)
		for j := 0; j < real_size; j++{
			//toSend := make([]byte, 1500)
			fmt.Println(cp.cwnd - len(ack_array))

			bs := fmt.Sprintf("%06d", seqn0)
			elt_list := ack_list{i,bs}
			ack_array = append(ack_array, elt_list) // configure all elements to send + to send again
			if (i == len(data)-1){ // ?
				data[i] = data[i][:last_len]
				i ++
				break
			}
			seqn0 ++
			i ++

		}

		for _, elt := range ack_array {
			fmt.Println("Sending...")
			fmt.Println(elt.index)
			toSend := append([]byte(elt.value), data[elt.index]...)
			pc.WriteTo(toSend, add)
				/*sbuffer := ""
				select{
				case <- time.After(1*time.Second):
						sbuffer = "erreur"
					case sbuffer = <- ch:
				}
				if(strings.Contains(sbuffer, bs)){
					break
				}
				*/
			}
		for {
			if (len(ack_array) == 0){
				fmt.Println("All ACK expected were received")
				cwnd_evolution(0,-1, cp)
				break
			}
			select{
				// case timeout to handle
				case ack_buffer, content := <- ch: // content false => buffer empty
				 fmt.Println("Data from channel...")
				 fmt.Println("Content or no longer content ?")
				 fmt.Println("Waiting from ACKs")
				 fmt.Println(ack_array)
				 fmt.Println(ack_buffer.n[3:])
				 exists, index := contains_find(ack_array, ack_buffer.n[3:]) // structure from channel (ack)
				 // fmt.Println(exists)
					if (content == false && len(ack_array) != 0){
						fmt.Print("Error was found, should resend")
						i = index
						cwnd_evolution(1, index, cp)
						break
					}
					if (exists){
						ack_array = ack_array[index+1:]
					}
			}
		}

		}
	fmt.Println("Fin d'envoi")
	pc.WriteTo([]byte("FIN"), add)
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
	cp := NewConn_param(float64(rtt))
	for {
		buffer := make([]byte, 1024)
		n, _, _ := pc.ReadFrom(buffer)
		fmt.Println("handle", port, add,"\n"+string(buffer[:n]), n)
		if sendFile(string(buffer[:n-1]), pc, add, cp){
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

	fmt.Println("Launching server")

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
				if time.Now().UnixNano() - v.toa > 1000000000{
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
