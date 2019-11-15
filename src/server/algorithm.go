package main

import (
	"fmt"
)

var cwnd int = 1
var congestion_type = "SS" // SS or CA (Congestion Avoidance)
const attenuation_coefficient float32 = 0.5
const incrementation_ca = 1

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
								cwnd = int(float32(cwnd+seq_failed[0])*attenuation_coefficient)
								if (congestion_type == "SS") {congestion_type="CA"}
							}
					case "CA":
						cwnd = int(float32(cwnd)*attenuation_coefficient)
				}

	}
}

// func cwnd_evolution(flag unint8){
// 	cwnd_evolution(flag, -1)
// }

// Define function that will increase cmd

func main() {
		cwnd_evolution(0);
		fmt.Println(cwnd)
		cwnd_evolution(0);
		fmt.Println(cwnd)
		cwnd_evolution(1, 2);
		fmt.Println(cwnd)
		cwnd_evolution(0);
		fmt.Println(cwnd)
		fmt.Println("Fin programme")
}
