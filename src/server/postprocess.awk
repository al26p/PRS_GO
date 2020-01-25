BEGIN {
	send = 0
	timedout = 0
	received = 0
	window_size = 1
}

{
    if($1 == "e") {
        send ++;
        window_size = $4
    }
    if($1 == "r") {
        received ++;
    }
    if($1 == "e") {
        timeoout ++;
    }

    printf("%6.2f %d %d %d %6.2f %6.2f %d \n", $2, send, received, timedout, received/send, dropped/send, window_size)


}
