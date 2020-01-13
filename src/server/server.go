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

const debug = true

const BufferSize = 1494

const attenuation_coefficient float32 = 0.5
const incrementation_ca = 1

// Potential RTT/RTO/SRTT evolution
const R = 0.0
// real initization will be done after first handshake (SYN-ACK -> ACK) will be the R variable (in ms)

const alpha = 1/8 // (RFC6298)
const beta = 1/4 // (RFC6298)
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
		RTO: r + K * r/2,
		cwnd: 1,
		congestion_type: "SS",
	}
	return cp
}

func logs(v ...interface{}) {
	if (debug == true){
		fmt.Println(v)
	}
}

func getPort() int {
	lock.Lock()
	defer lock.Unlock()
	for k := range portList{
		delete(portList, k)
		return k
	}
	logs("Plus de ports disponibles")
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
		logs("erreur avec le port " + port)
		return -1
	}
}

func testPorts(portMin int, portMax int) {
	for p:=portMin; p<=portMax; p++ {
		releasePort(testPort(p))
	}
}

func update_time_mesure(new_measure float64, cp *conn_param) {
	cp.SRTT = (1-alpha)*cp.SRTT + alpha*new_measure
	cp.RTTVAR = (1-beta)*cp.RTTVAR + beta*math.Abs(new_measure-cp.SRTT)
	cp.RTO = cp.SRTT + K*cp.RTTVAR
}


func cwnd_evolution (flag int, seq_failed int, cp *conn_param){
	/*
		Function that will deal with the evolution of our congestion window and
		that will handle the switch from slow start to congestion avoidance and
		so recalculate our new cwnd

		flag : indicates whether there was an error (ACK not received) in a RTO

		---- 0 => everything received
		---- 1 => error

		AIMD implementation
	*/
	logs("Evolution of cwnd")
	switch flag {
		case 0:
			switch (cp.congestion_type){
				case "SS":
					logs("SS WINDOW")
					cp.cwnd *= 2

				case "CA":
					logs("From SS to CA")
					cp.cwnd += incrementation_ca
			}
		case 1:
			switch (cp.congestion_type){
					case "SS":
						if (seq_failed > 0){
							  logs("To CA")
								cp.cwnd = int(float32(cp.cwnd)*attenuation_coefficient)+1 //index ?
								cp.congestion_type="CA"
							}				// case timeout to handle

					case "CA":
						cp.cwnd = int(float32(cp.cwnd)*attenuation_coefficient)+1

					default:
						cp.RTO*=2
						logs("Congestion => increase RTO (by 2)")
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
		logs("Error while openning", absFile)
		return nil, 0
	}
	defer f.Close()
	data := make([][]byte, 0)
	n := 0
	m := 0
	for {
		d := make([]byte, BufferSize)
		n, _ = f.Read(d)
		//logs("Nombre de bytes lus", n)
		if n == 0{
			break
		}
		m = n
		if err != nil {
			if err != io.EOF {
				logs(err)
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

func sendFile(file string, pc net.PacketConn, add net.Addr, cp *conn_param) bool {
	var last_ack = ""
	data, last_len := readFile(file) // data : array of data size of buffer
	logs("Data longeur ",len(data))
	seqn0, i := 000001, 0
	ch := make(chan ack, 1000)
	go readpc(pc, ch)
	var ack_array []ack_list // Initial array with all expected ACKs
	var next_id = 0
	backoff := 1 //backoff when timing out
	for i < len(data){
		logs("Taille de la fenêtre ", *cp)
		for j := 0; j < cp.cwnd; j++{
			//toSend := make([]byte, 1500)
			bs := fmt.Sprintf("%06d", seqn0)
			elt_list := ack_list{i,bs}
			ack_array = append(ack_array, elt_list) // configure all elements to send + to send again

			if (i == len(data)-1){ // si dermier packet à envoyer
				data[i] = data[i][:last_len]
				seqn0 ++
				i ++
				break
			}
			seqn0 ++
			i ++
		}

		for _, elt := range ack_array { //for each in batch
			logs("Sending", elt.index, "...")
			toSend := append([]byte(elt.value), data[elt.index]...)
			pc.WriteTo(toSend, add)
			//TODO : trace when sendend to know when timeouts or we can evaluate each rtt

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

		for { //Checking ACKs loop
			exit := 0
			if (len(ack_array) == 0){ //Ya R, what is done is done
				logs("All ACK expected were received")
				cwnd_evolution(0,-1, cp)
				break
			}
			select{
				case ack_buffer, content := <- ch: // content false => buffer empty
				backoff = 1 //resetting backoff value
				 logs("Getting data from channel")
				 logs("Content or no longer content ?", content)
				 logs("Waiting from ACKs :")
				 logs(ack_array)
				 logs("Trading with ACK", ack_buffer.n[3:]) //ACK be like ACK000124 so [3:]
				 exists, index := contains_find(ack_array, ack_buffer.n[3:]) // structure from channel (ack)
				 // logs(exists)
					if (content == false && len(ack_array) != 0){
						fmt.Print("Error was found, should resend")
						i = index
						ack_array = nil
						cwnd_evolution(1, index, cp)
						exit = 1
						break
					}

					if (ack_buffer.n == last_ack){
						logs("Similar ACKs revoyer.")
						last_id,_ := strconv.Atoi(ack_buffer.n[3:])
						toSend := append([]byte(fmt.Sprintf("%06d", last_id+1)), data[last_id]...)
						pc.WriteTo(toSend, add)
						for {
							fexit := false
							select{
							case ack_ans, _ := <- ch:
								  to_compare,_ := strconv.Atoi(ack_ans.n[3:])
									if (to_compare != last_id){
									next_id,_ = strconv.Atoi(ack_ans.n[3:])
									logs("On recommence au paquet ",next_id)
									i = next_id
									seqn0 = i+1
									fexit = true
									ack_array = nil
									cwnd_evolution(1, 1 ,cp) // Congestion avoidance
									break
								}
							//time.sleep du RTT
						}
						if (fexit) {
							break
						}
					}
					}
					if (exists){
						ack_array = ack_array[index+1:]
						last_ack = ack_buffer.n
					}
				//case <- time.After(math.Round(cp.RTO * time.Second): //First etch of timeout
			case <- time.After( time.Duration(int(cp.RTO)) * time.Nanosecond):
					logs("Timed out - backoff:", backoff)
					cwnd_evolution(1, ack_array[0].index, cp)
					cp.RTO = cp.RTO * math.Pow(float64(2), float64(backoff))
					backoff ++
					ack_array = nil
					exit = 1
					break
			}
			if (exit == 1){
				break
			}
		}}

	logs("Fin d'envoi")
	pc.WriteTo([]byte("FIN"), add)
	return true
}


func handleClient(add net.Addr, port int, c chan int64){
	defer releasePort(port)
	defer logs("FIN Transmission")
	pc, err := net.ListenPacket("udp", "0.0.0.0:"+strconv.Itoa(port))
	if err != nil {
		log.Fatal(err)
	}
	defer pc.Close()
	var rtt int64
	select {
	case rtt =<-c:
		logs("ok - RTT", rtt, "ns (1ms = 1 000 000ns)")
		if rtt == 0 {
			logs("Deleted")
			return
		}

	case <- time.After(10 * time.Second):
		logs("Deleted")
		return
	}
	cp := NewConn_param(float64(rtt))
	for {
		buffer := make([]byte, 1024)
		n, _, _ := pc.ReadFrom(buffer)
		logs("handle", port, add,"\n"+string(buffer[:n]), n)
		if sendFile(string(buffer[:n-1]), pc, add, &cp){
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

	logs("Testing ports")
	portList = make(map[int]void)
	testPorts(1000,9999)
	logs("Initial portList has been set")

	logs("Launching server")

	pc, err := net.ListenPacket("udp", "0.0.0.0:"+port)
	if err != nil {
		log.Fatal(err)
	}
	defer pc.Close()

	for {
		logs("ACK waiting :", len(addrWait))
		buffer := make([]byte, 1024)
		_, addr, err := pc.ReadFrom(buffer)
		if err != nil {
			log.Fatal(err)
		}
		in := string(buffer)
		add := addr.String()
		logs(in ,addr)
		if strings.Contains(in, "SYN") {
			for k, v := range addrWait{
				logs(k,time.Now().UnixNano()-  v.toa )
				if time.Now().UnixNano() - v.toa > 1000000000{
					logs("Deleting", k)
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
					c: ch,
				}
				addrWait[add] = s
				pc.WriteTo([]byte("SYN-ACK"+strconv.Itoa(p)), addr)
			}

		}else if el, found := addrWait[add]; strings.Contains(in, "ACK") && found{
			logs("Got ACK")
			el.c <- time.Now().UnixNano() - el.toa //RTT dans la goroutine
			close(el.c)
			delete(addrWait, add)
		}else{
			logs(addrWait)
			continue
		}
	}
}
