BEGIN {
	send = 0
	timedout = 0
	received = 0
	window_size = 1
	rto = 0
}

{
    if($1 == "e") {
        send ++;
        window_size = $5
				rto = $6
    }
    if($1 == "r") {
        received ++;
    }
    if($1 == "e") {
        timedout ++;
    }

    printf("%6.4f %d %d %d %d %10.0f \n", $2, send, received, timedout, window_size, rto/1000)


}
