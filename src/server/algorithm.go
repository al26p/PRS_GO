package main

import (
	"fmt"
	"math"
)

var cwnd int = 1
var congestion_type = "SS" // SS or CA (Congestion Avoidance)
const attenuation_coefficient float32 = 0.5
const incrementation_ca = 1

// Potential RTT/RTO/SRTT evolution
const R = 0.0
// real initization will be done after first handshake (SYN-ACK -> ACK) will be the R variable (in ms)

const alpha = 1/8 // (RFC6298)
const beta = 1/4 // (RFC6298)
const granularity = 1// granularity of the clock we'll use (think that time library works in ms)
const K = 4 // (RFC6298)

// R must be a float otherwise change it !
var SRTT = R //array for each client ?
var RTTVAR = R/2 // Estimate the potential variation of the RTT
var RTO = SRTT + math.Max(granularity, K*RTTVAR)


func update_time_mesure(new_measure float64) {
	RTTVAR = (1-beta)*RTTVAR + beta*math.Abs(SRTT-new_measure)
	SRTT = (1-alpha)*SRTT + alpha*new_measure
	RTO = SRTT + math.Max(granularity, K*RTTVAR)
}


func cwnd_evolution (flag int, seq_failed ...int){
	/*
		Function that will deal with the evolution of our congestion window and 
		that will handle the switch from slow start to congestion avoidance and 
		so recalculate our new cwnd

		flag : indicates whether there was an error (ACK not received) in a RTO

		---- 0 => everything received
		---- 1 => error 

		AIMD implementation
	*/	
	switch flag {
		case 0:
			switch congestion_type{
				case "SS":
					cwnd *= 2
				case "CA":
					cwnd += incrementation_ca	
			}
		case 1:
			switch congestion_type{
					case "SS":
						if (len(seq_failed) > 0){
								cwnd = int(float32(cwnd+seq_failed[0])*attenuation_coefficient)+1
								if (congestion_type == "SS") {congestion_type="CA"}
							}
					case "CA":
						cwnd = int(float32(cwnd)*attenuation_coefficient)+1

					default:
						RTO*=2
						fmt.Println("Congestion => increase RTO (by 2)")	
				}

	}
}

// func cwnd_evolution(flag unint8){
// 	cwnd_evolution(flag, -1)
// }

// Define function that will increase cmd

func main() {
		// Test functions														
		fmt.Println("Fin programme")
}
