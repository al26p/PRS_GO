package main

import (
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

/*---------------------- TYPE DEF --------------------------*/

type void struct{}

type waitParam struct {
	toa int64
	c   chan int64
}

type ack struct {
	n   string
	toa int64
}

type ackList struct {
	index int
	value string
}

type connParam struct {
	SRTT           float64
	RTTVAR         float64
	RTO            float64
	cwnd           int
	congestionType string
	lastRtt        []float64
}

func newConnParam(r float64) connParam {
	cp := connParam{
		SRTT:           r,
		RTTVAR:         r / 2,
		RTO:            r + K*r/2,
		cwnd:           1,
		congestionType: "SS",
		lastRtt:        []float64{0.0},
	}
	return cp
}

/*---------------------- VARIABLES --------------------------*/

const (
	RefValue               = 10
	alpha                  = 1 / 8
	beta                   = 1 / 4
	K                      = 3
	attenuationCoefficient = 0.5
	incrementationCa       = 1
	BufferSize             = 1494
	minWnd                 = 0
	maxTimeout 			   		 = 100000000
)

var (
	debug    bool                 //is server in debug mode?
	portList map[int]void         //List of ports available
	addrWait map[string]waitParam //List of client assigned but not not connected
	lock     sync.Mutex           //Mutex for acquiring portList
)

/*---------------------- FUNCTIONS --------------------------*/

func logs(v ...interface{}) { //Simple logging function
	if debug == true {
		fmt.Println(v)
	}
}

func getPort() int { //Get random port from map
	lock.Lock()
	defer lock.Unlock()
	for k := range portList {
		delete(portList, k)
		return k
	}
	logs("Plus de ports disponibles")
	return 0
}

func releasePort(port int) { //Return port to map
	if port > 0 {
		lock.Lock()
		defer lock.Unlock()
		portList[port] = void{}
	}
}

func testPort(p int) int { //Test if port is available
	port := strconv.Itoa(p)
	pc, err := net.ListenPacket("udp", "0.0.0.0:"+port)
	if err == nil {
		pc.Close()
		return p
	} else {
		logs("Erreur avec le port " + port)
		return -1
	}
}

func testPorts(portMin int, portMax int) { //Test all port of given range
	for p := portMin; p <= portMax; p++ {
		releasePort(testPort(p))
	}
}

func getStandardDeviation(timesMeasured []float64) float64 {
	somme := 0.0
	logs("My array length: ", len(timesMeasured))
	var standardDeviation float64
	longueur := len(timesMeasured)
	for i := 0; i < longueur; i++ {
		somme += timesMeasured[i]
	}
	var moyenne = somme / float64(longueur)
	for i := 0; i < longueur; i++ {
		standardDeviation += math.Pow(float64(timesMeasured[i]-moyenne), 2)
	}
	return math.Sqrt(standardDeviation / float64(longueur))
}

func addToCpLastRtt(value float64, cp *connParam) {
	if len(cp.lastRtt) < RefValue {
		if cp.lastRtt[0] == 0 {
			cp.lastRtt[0] = value
		} else {
			cp.lastRtt = append(cp.lastRtt, value)
		}
	} else {
		copy(cp.lastRtt[1:], cp.lastRtt[0:])
		cp.lastRtt[0] = value
		cp.RTTVAR = getStandardDeviation(cp.lastRtt)
	}
}

func updateTimeMesure(newMeasure float64, cp *connParam) {
	addToCpLastRtt(newMeasure, cp)
	cp.SRTT = (1-alpha)*cp.SRTT + alpha*newMeasure
	cp.RTO = cp.SRTT + K*cp.RTTVAR
}

func cwndEvolution(flag int, seqFailed int, cp *connParam) {
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
		switch cp.congestionType {
		case "SS":
			logs("SS WINDOW")
			cp.cwnd *= 2

		case "CA":
			logs("In CA")
			cp.cwnd += incrementationCa
		}
	case 1:
		switch cp.congestionType {
		case "SS":
			if seqFailed > 0 {
				logs("To CA")
				cp.cwnd = 1 //int(float32(cp.cwnd)*attenuationCoefficient) //index ?
				cp.congestionType = "CA"
			} // case timeout to handle

		case "CA":
			cp.cwnd = 1//int(float32(cp.cwnd)*attenuationCoefficient)
		}

	}
	cp.cwnd = int(math.Min(float64(cp.cwnd), float64(300)))
}

func containsFind(a []ackList, x string) (bool, int) {
	for i, n := range a {
		if strings.Compare(x, n.value) == 0 {
			return true, i
		}
	}
	return false, 0
}

// https://kgrz.io/reading-files-in-go-an-overview.html
func readFile(file string) ([][]byte, int, int64) {
	//file, _ = regexp.MatchString()
	//file = "coucou "
	absFile, _ := filepath.Abs(file)
	f, err := os.Open(absFile)
	if err != nil {
		logs("Error while openning", absFile)
		return nil, 0, 0
	}

	fInfo, _ := f.Stat()
	fI := fInfo.Size()
	defer f.Close()
	data := make([][]byte, 0)
	n := 0
	m := 0
	for {
		d := make([]byte, BufferSize)
		n, _ = f.Read(d)
		//logs("Nombre de bytes lus", n)
		if n == 0 {
			break
		}
		m = n
		if err != nil {
			if err != io.EOF {
				logs(err)
				return nil, 0, 0
			}
			data = append(data, d)
			break
		}
		data = append(data, d)
	}
	return data, m, fI
}

func readpc(pc net.PacketConn, ch chan ack, logfile *string, timelog time.Time, q chan struct{}) {
	lastGet := 0
	for {
		select {
		case <-q:
			return
		default:
			buffer := make([]byte, 100)
			n, _, _ := pc.ReadFrom(buffer)
			if n > 0 {
				aa, _ := strconv.Atoi(string(buffer[3:n-1]))
				if  aa >= lastGet {
					ch <- ack{string(buffer[:n-1]), time.Now().UnixNano()}
					*logfile += "r " + time.Now().Sub(timelog).String() + " " + string(buffer[3:n-1]) + " " + strconv.Itoa(n) + " \n"
					lastGet = aa
					fmt.Println("LAST GET", lastGet)
				}
			}
		}
	}
}

func sendFile(file string, pc net.PacketConn, add net.Addr, cp *connParam) bool {
	var lastAck = ""
	var logOut = ""                           //logging will be like : time index buffer_size
	data, lastLen, fileInfo := readFile(file) // data : array of data size of buffer
	logs("Data longeur ", len(data))
	seqn0, i := 000001, 0
	ch := make(chan ack, 1000)
	startlog := time.Now()
	quit := make(chan struct{})
	go readpc(pc, ch, &logOut, startlog, quit)
	var ackArray []ackList // Initial array with all expected ACKs
	var nextId = 0
	var rttList = make(map[string]int64)
	backoff := 1 //backoff when timing out
	for i < len(data) {
		logs("Taille de la fenêtre ", *cp)
		for j := 0; j < cp.cwnd+minWnd-len(ackArray); j++ {
			//toSend := make([]byte, 1500)
			bs := fmt.Sprintf("%06d", seqn0)
			eltList := ackList{i, bs}
			ackArray = append(ackArray, eltList) // configure all elements to send + to send again

			if i == len(data)-1 { // si dernier packet à envoyer
				data[i] = data[i][:lastLen]
				seqn0++
				i++
				break
			}
			seqn0++
			i++
		}
		logs(ackArray)
		for _, elt := range ackArray { //for each in batch

			logs("Sending", elt.index+1, "...")
			toSend := append([]byte(elt.value), data[elt.index]...)
			rttList[elt.value] = time.Now().UnixNano()
			_, _ = pc.WriteTo(toSend, add)

			logOut += "e " + time.Now().Sub(startlog).String() + " " + elt.value + " " + strconv.Itoa(len(ackArray)) + " \n"

		}
		updaye := 0
		for { //Checking ACKs loop
			exit := 0
			if len(ackArray) == 0 { //Ya R, what is done is done
				logs("All ACK expected were received")
				cwndEvolution(0, -1, cp)
				break
			}
			select {
			case ackBuffer, content := <-ch: // content false => buffer empty
				backoff = 1 //resetting backoff value
				logs("Getting data from channel")
				logs("Content or no longer content ?", content)
				logs("Waiting from ACKs :")
				logs(ackArray)
				logs("Trading with ACK", ackBuffer.n[3:]) //ACK be like ACK000124 so [3:]
				//fmt.Println("ACK_BUFFER.TOA : ", ack_buffer.toa)
				//fmt.Println("RTT LIST ", rtt_list[ack_buffer.n[3:]])
				updaye ++
				if rttList[ackBuffer.n[3:]] != 0 && updaye > 100{
					updateTimeMesure(float64(ackBuffer.toa-rttList[ackBuffer.n[3:]]), cp) // nul germain
					logs("RTO : ", cp.RTO)
					updaye = 0
				}
				logs("RTT for this packet is", ackBuffer.toa-rttList[ackBuffer.n[3:]])
				delete(rttList, ackBuffer.n[3:])
				exists, index := containsFind(ackArray, ackBuffer.n[3:]) // structure from channel (ack)
				logs("Last ACK sent was ", lastAck, "and this one ", ackBuffer.n)
				resBuffer, _ := strconv.Atoi(ackBuffer.n[3:])
				resAckarray, _ := strconv.Atoi(ackArray[len(ackArray)-1].value)
				// logs(exists)
				if len(ackArray) != 0 && content == true && resBuffer > resAckarray {
					logs("Packet way beyond !")
					i = resBuffer - 1
					seqn0 = i + 1
					ackArray = nil
					cwndEvolution(1, index, cp)
					exit = 1
					logs("Envoi du paquet pas réussi")
					break
				}
				received := false
				for ackBuffer.n == lastAck && !(received) {
					logs("Similar ACKs revoyer.")
					lastId, _ := strconv.Atoi(ackBuffer.n[3:])
					toSend := append([]byte(fmt.Sprintf("%06d", lastId+1)), data[lastId]...)
					logs("Spot error about to send again...packet ", lastId+1)
					_, _ = pc.WriteTo(toSend, add)

					logOut += "* " + time.Now().Sub(startlog).String() + " " + strconv.Itoa(lastId+1) + " 1 \n"
					for {
						fexit := false
						select {
						case ackAns, _ := <-ch:
							logs(content)
							toCompare, _ := strconv.Atoi(ackAns.n[3:])
							if toCompare != lastId {
								nextId, _ = strconv.Atoi(ackAns.n[3:])
								logs("On recommence au paquet ", nextId+1)
								i = nextId
								seqn0 = i + 1
								fexit = true
								ackArray = nil
								received = true
								lastAck = ackBuffer.n
								logs("Value of i en sortie ", i)
								cwndEvolution(1, 1, cp) // Congestion avoidance
								break
							}
						default:
							received = false
							time.Sleep(time.Duration(int(cp.RTO)) * time.Nanosecond)
							fexit = true
							break

							//time.sleep du RTT
						}
						if fexit {
							break
						}
					}
				}
				if exists && len(ackArray) > 0 {
					ackArray = ackArray[index+1:]
					lastAck = ackBuffer.n
				}
				//case <- time.After(math.Round(cp.RTO * time.Second): //First etch of timeout
			case <-time.After(time.Duration(int(math.Min(math.Max(cp.RTO, 100), maxTimeout))) * time.Nanosecond):
				logs("Timed out - backoff:", backoff)
				cwndEvolution(1, ackArray[0].index, cp)
				cp.RTO = cp.RTO * math.Pow(float64(2), float64(backoff))
				i = ackArray[0].index
				seqn0 = i + 1
				ackArray = nil
				exit = 1
				logs(ackArray)
				break
			}
			if exit == 1 {
				break
			}
		}
	}

	_, _ = pc.WriteTo([]byte("FIN"), add)
	end := time.Now().Sub(startlog)
	fmt.Println("Fin d'envoi")

	logOut += "/ " + end.String() + " 999999 0 \n"

	debit := (float32(fileInfo) / float32(end/time.Millisecond)) * 1000
	fmt.Println("Débit is : ", debit, "o/s")
	fmt.Println(debit/1000000, "Mo/s")

	logOut += "$ " + fmt.Sprintf("%f", debit) + " 999999 0 \n"

	var re = regexp.MustCompile(`([.-z]*) ([0-9]+).([0-9]+)µs ([ -Z]*)`)
	logOut = re.ReplaceAllString(logOut, `$1 0.$2${3}ms $4`)
	re = regexp.MustCompile(`([ -z]+)ms([ -z]*)`)
	logOut = re.ReplaceAllString(logOut, `$1 ms$2`)
	f, _ := os.Create("logs/log_send_" + file + "_" + time.Now().String())
	defer f.Close()
	_, _ = f.WriteString(logOut)
	_ = f.Sync()

	close(quit)
	return true
}

func handleClient(add net.Addr, port int, c chan int64) {
	defer releasePort(port)
	defer fmt.Println("FIN Transmission", add, port)
	pc, err := net.ListenPacket("udp", "0.0.0.0:"+strconv.Itoa(port))
	if err != nil {
		log.Fatal(err)
	}
	defer pc.Close()
	var rtt int64
	select {
	case rtt = <-c:
		fmt.Println("ok - RTT", rtt, "ns (1ms = 1 000 000ns)")
		if rtt == 0 {
			logs("Deleted")
			return
		}

	case <-time.After(10 * time.Second):
		logs("Deleted")
		return
	}
	cp := newConnParam(float64(rtt * 4))
	for {
		buffer := make([]byte, 1024)
		n, _, _ := pc.ReadFrom(buffer)
		fmt.Println("handle", port, add, "\n"+string(buffer[:n]), n)
		if sendFile(string(buffer[:n-1]), pc, add, &cp) {
			return
		}
	}

}

func main() {
	args := os.Args[1:]
	port := "8080"
	//Init with args
	if len(args) >= 1 {
		port = args[0]
	}
	if len(args) >= 2 {
		debug = args[1] == "true"
	}

	fmt.Println("Testing ports")
	portList = make(map[int]void)
	addrWait = make(map[string]waitParam)
	testPorts(1000, 9999)
	fmt.Println("Initial portList has been set")

	fmt.Println("Launching server")
	pc, err := net.ListenPacket("udp", "0.0.0.0:"+port)
	if err != nil {
		log.Fatal(err)
	}
	defer pc.Close()
	fmt.Println("Server is awaiting connections.")

	for {
		logs("Client(s) waiting :", len(addrWait))
		buffer := make([]byte, 1024)
		_, addr, err := pc.ReadFrom(buffer)
		if err != nil {
			log.Fatal(err)
		}
		in := string(buffer)
		add := addr.String()
		logs(fmt.Sprintf("Got %s from %s", in, addr))
		if strings.Contains(in, "SYN") {
			for k, v := range addrWait { //Test if client has been here for too long
				logs(k, time.Now().UnixNano()-v.toa)
				if time.Now().UnixNano()-v.toa > 1000000000 {
					logs("Deleting", k)
					close(v.c)
					delete(addrWait, k)
				}
			}
			p := getPort()
			if testPort(p) > 0 {
				ch := make(chan int64)
				s := waitParam{
					toa: time.Now().UnixNano(),
					c:   ch,
				}
				go handleClient(addr, p, ch)
				addrWait[add] = s
				_, _ = pc.WriteTo([]byte("SYN-ACK"+strconv.Itoa(p)), addr)
			}

		} else if el, found := addrWait[add]; strings.Contains(in, "ACK") && found {
			logs("Got ACK from", add)
			el.c <- time.Now().UnixNano() - el.toa //RTT dans la goroutine
			close(el.c)
			delete(addrWait, add)
		} else {
			logs(addrWait)
			continue
		}
	}
}
