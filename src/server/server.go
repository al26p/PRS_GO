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

type void struct{}

type ack struct {
	n   string
	toa int64
}

type waitParam struct {
	toa int64
	c   chan int64
}

type ack_list struct {
	index int
	value string
}

var (
	lock     sync.Mutex
	portList map[int]void
	m        void
)

var debug = true
var calculate_RTT = false
const BufferSize = 1494//9500
const attenuation_coefficient float32 = 0.5
const incrementation_ca = 1
const REF_VALUE = 5
const min_wnd = 0
// Potential RTT/RTO/SRTT evolution

// real initization will be done after first handshake (SYN-ACK -> ACK) will be the R variable (in ms)

const alpha = 0.125 // (RFC6298)
const beta = 0.25  // (RFC6298)
const K = 4      // (RFC6298)

type conn_param struct {
	SRTT            float64
	RTTVAR          float64
	RTO             float64
	cwnd            int
	congestion_type string
	last_rtt				[]float64
}

func NewConn_param(r float64) conn_param {
	cp := conn_param{
		SRTT:            r,
		RTTVAR:          r / 2,
		RTO:             r + K*r/2,
		cwnd:            1,
		congestion_type: "SS",
		last_rtt:        []float64{0.0},
	}
	return cp
}

func logs(v ...interface{}) {
	if debug == true {
		fmt.Println(v)
	}
}

func getPort() int {
	lock.Lock()
	defer lock.Unlock()
	for k := range portList {
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
	} else {
		logs("erreur avec le port " + port)
		return -1
	}
}

func testPorts(portMin int, portMax int) {
	for p := portMin; p <= portMax; p++ {
		releasePort(testPort(p))
	}
}

//const alpha = 1/8 // (RFC6298)
//const beta = 1/4 // (RFC6298)
//const K = 8 // (RFC6298)

func get_standard_deviation(times_measured []float64) float64{
	somme := 0.0;
	logs("My array length: ",len(times_measured))
	var standard_deviation float64;
	longueur := len(times_measured)
	for i := 0; i < longueur; i ++{
		somme += times_measured[i]
	}
	var moyenne = somme/float64(longueur);
	for i:=0; i< longueur; i ++{
		standard_deviation += math.Pow(float64(times_measured[i]-moyenne), 2);
	}
	return math.Sqrt(standard_deviation/float64(longueur));
}

func add_to_cp_last_rtt(value float64, cp *conn_param){
		if (len(cp.last_rtt) < REF_VALUE){
			if (cp.last_rtt[0] == 0){
				cp.last_rtt[0] = value
			}else{
				cp.last_rtt = append(cp.last_rtt, value);
	}} else{
		copy(cp.last_rtt[1:], cp.last_rtt[0:])
		cp.last_rtt[0] = value
		cp.RTTVAR =  get_standard_deviation(cp.last_rtt)
	}
}


func update_time_mesure(new_measure float64, cp *conn_param) {
	add_to_cp_last_rtt(new_measure, cp)
	cp.SRTT = (1-alpha)*cp.SRTT + alpha*new_measure
	cp.RTO = cp.SRTT+K*cp.RTTVAR
}

func contains_value(arr []float64, str float64) bool {
   for a:=0; a < len(arr); a ++{
      if arr[a] == str {
         return true
      }
   }
   return false
}

func cwnd_evolution(flag int, seq_failed int, cp *conn_param) {
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
		switch cp.congestion_type {
		case "SS":
			logs("SS WINDOW")
			cp.cwnd *= 2

		case "CA":
			logs("In CA")
			cp.cwnd += incrementation_ca
		}
	case 1:
		switch cp.congestion_type {
		case "SS":
			if seq_failed > 0 {
				logs("To CA")
				cp.cwnd = int(float32(cp.cwnd)*attenuation_coefficient) //index ?
				cp.congestion_type = "CA"
			} // case timeout to handle

		case "CA":
			cp.cwnd = int(float32(cp.cwnd)*attenuation_coefficient)
		}

	}
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

	f_info, _ := f.Stat()
	f_i := f_info.Size()
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
	return data, m, f_i
}

func readpc(pc net.PacketConn, ch chan ack, logfile *string, timelog time.Time, q chan struct{}) {
	lastGet := 0
	for {
		select{
		case <- q:
			return
		default:
			buffer := make([]byte, 100)
			n, _, _ := pc.ReadFrom(buffer)
			if n > 0 {
				aa, _ := strconv.Atoi(string(buffer[3:n-1]))
				if aa >= lastGet {
					ch <- ack{string(buffer[:n-1]), time.Now().UnixNano()}
					*logfile += "r " + time.Now().Sub(timelog).String() + " " + string(buffer[3:n-1]) + " " + strconv.Itoa(n) + " \n"
					lastGet = aa
				}
			}
		}
	}
}

/*func remove(slice []int, s int) []int {
    return append(slice[:s], slice[s+1:]...)
}
*/

func contains_find(a []ack_list, x string) (bool, int) {
	for i, n := range a {
		if strings.Compare(x, n.value) == 0 {
			return true, i
		}
	}
	return false, 0
}

func sendFile(file string, pc net.PacketConn, add net.Addr, cp *conn_param) bool {
	var last_ack = ""
	var log_out = ""                            //logging will be like : time index buffer_size
	data, last_len, file_info := readFile(file) // data : array of data size of buffer
	logs("Data longeur ", len(data))
	seqn0, i := 000001, 0
	ch := make(chan ack, 1000)
	startlog := time.Now()
	quit := make(chan struct{})
	go readpc(pc, ch, &log_out, startlog, quit)
	var ack_array []ack_list // Initial array with all expected ACKs
	var next_id = 0
	var rtt_list = make(map[string]int64)
	for i < len(data) {
		logs("Taille de la fenêtre ", *cp)
		for j := 0; j < cp.cwnd+min_wnd; j++ {
			bs := fmt.Sprintf("%06d", seqn0)
			elt_list := ack_list{i, bs}
			ack_array = append(ack_array, elt_list) // configure all elements to send + to send again

			if i == len(data)-1 { // si dernier packet à envoyer
				data[i] = data[i][:last_len]
				seqn0++
				i++
				break
			}
			seqn0++
			i++
		}

		fmt.Println(len(ack_array))

		for _, elt := range ack_array { //for each in batch
			logs("Sending", elt.index+1, "...")
			toSend := append([]byte(elt.value), data[elt.index]...)
			rtt_list[elt.value] = time.Now().UnixNano()
			pc.WriteTo(toSend, add)
			log_out += "e " + time.Now().Sub(startlog).String() + " " + elt.value + " " + strconv.Itoa(len(ack_array)) + " \n"
		}

		for { //Boucle d'écoute
			exit := 0

			if len(ack_array) == 0 { //Ya R, what is done is done
				logs("All ACK expected were received")
				cwnd_evolution(0, -1, cp)
				break
			}

			select {
			case ack_buffer, content := <-ch: // content false => buffer empty
				logs("Getting data from channel")
				logs("Content or no longer content ?", content)
				logs("Waiting from ACKs :")
				logs(ack_array)
				logs("Trading with ACK", ack_buffer.n[3:]) //ACK be like ACK000124 so [3:]

				if (rtt_list[ack_buffer.n[3:]] != 0){
					update_time_mesure(float64(ack_buffer.toa-rtt_list[ack_buffer.n[3:]]), cp) // nul germain
				}

				logs("RTT for this packet is", ack_buffer.toa-rtt_list[ack_buffer.n[3:]])
				delete(rtt_list, ack_buffer.n[3:])

				exists, index := contains_find(ack_array, ack_buffer.n[3:]) // structure from channel (ack)

				seq_recue, _ := strconv.Atoi(ack_buffer.n[3:])
				seq_max, _ := strconv.Atoi(ack_array[len(ack_array)-1].value)

				if seq_recue > seq_max {
					i = seq_recue
					seqn0 = seq_recue + 1
					ack_array = nil
					cwnd_evolution(1, index, cp)
					logs("Envoi du paquet pas réussi")
					exit = 1
					break
				}

				for ack_buffer.n == last_ack {
					logs("Similar ACKs revoyer.")
					toSend := append([]byte(fmt.Sprintf("%06d", seq_recue+1)), data[seq_recue]...)
					logs("Spot error about to send again...packet ", seq_recue+1)
					pc.WriteTo(toSend, add)
					log_out += "* " + time.Now().Sub(startlog).String() + " " + strconv.Itoa(seq_recue+1) + " 1 \n"
					for {
						fexit := false
						select {
						case ack_ans, _ := <-ch:
							if ack_ans.n != ack_buffer.n {
								next_id, _ = strconv.Atoi(ack_ans.n[3:])
								logs("On recommence au paquet ", next_id+1)
								i = next_id
								seqn0 = i + 1
								fexit = true
								ack_array = nil
								last_ack = ack_ans.n
								logs("Value of i en sortie ", i)
								cwnd_evolution(1, 1, cp) // Congestion avoidance
								break
							}
						case <-time.After(time.Duration(int(math.Max((cp.RTO), 100))) * time.Nanosecond):
							fexit = true
							break	//time.sleep du RTT
						}
						if fexit {
							break
						}
					}
					//FOR EXITED BC CONDITION FALSE
				}

				if exists && len(ack_array) > 0 {
					ack_array = ack_array[index+1:]
					last_ack = ack_buffer.n
				}
			case <-time.After(time.Duration(int(math.Max((cp.RTO), 100))) * time.Nanosecond):
				cwnd_evolution(1, ack_array[0].index, cp)
				//cp.RTO = cp.RTO * math.Pow(float64(2), float64(backoff))
				i = ack_array[0].index
				seqn0 = i + 1
				ack_array = nil
				exit = 1
				logs(ack_array)
				break

			} // FIN SELECT
			if exit == 1 {
				break
			}

	} //FIN BOUCLE ECOUTE
}//FIN I

	pc.WriteTo([]byte("FIN"), add)
	end := time.Now().Sub(startlog)
	fmt.Println("Fin d'envoi")

	log_out += "/ " + end.String() + " 999999 0 \n"

	debit := (float32(file_info) / float32(end/time.Millisecond)) * 1000
	fmt.Println("Débit is : ", debit, "o/s")
	fmt.Println(debit/1000000, "Mo/s")

	log_out += "$ " + fmt.Sprintf("%f", debit) + " 999999 0 \n"

	var re = regexp.MustCompile(`([.-z]*) ([0-9]+).([0-9]+)µs ([ -Z]*)`)
	log_out = re.ReplaceAllString(log_out, `$1 0.$2${3}ms $4`)
	re = regexp.MustCompile(`([ -z]+)ms([ -z]*)`)
	log_out = re.ReplaceAllString(log_out, `$1 ms$2`)
	f, _ := os.Create("logs/log_send_" + file + "_" + time.Now().String())
	defer f.Close()
	f.WriteString(log_out)
	f.Sync()

	close(quit)
	return true
}

func handleClient(add net.Addr, port int, c chan int64) {
	defer releasePort(port)
	defer fmt.Println("FIN Transmission")
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
	cp := NewConn_param(float64(rtt)*125)
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
	addrWait := make(map[string]waitParam)

	if len(args) == 1 {
		port = args[0]
	}

	if len(args) == 2 {
		debug = args[1] == "true"
	}

	fmt.Println("Testing ports")
	portList = make(map[int]void)
	testPorts(1000, 9999)
	fmt.Println("Initial portList has been set")

	fmt.Println("Launching server")

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
		logs(in, addr)
		if strings.Contains(in, "SYN") {
			for k, v := range addrWait {
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
				go handleClient(addr, p, ch)
				s := waitParam{
					toa: time.Now().UnixNano(),
					c:   ch,
				}
				addrWait[add] = s
				pc.WriteTo([]byte("SYN-ACK"+strconv.Itoa(p)), addr)
			}

		} else if el, found := addrWait[add]; strings.Contains(in, "ACK") && found {
			logs("Got ACK")
			el.c <- time.Now().UnixNano() - el.toa //RTT dans la goroutine
			close(el.c)
			delete(addrWait, add)
		} else {
			logs(addrWait)
			continue
		}
	}
}
