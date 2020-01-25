BEGIN {
	start = 5 # second
	timestep = 0.5 # second
	nextstep = start + timestep;
}

{

	if($2 > nextstep) {

		for(f=0; f<=flows; f++) {

			if(rx[f] > 0) {
				avg_delay = delay[f]/rx[f];
			}
			else {
				avg_delay = 0;
			}

			printf("%6.2f %4d %8d %8d %10.6f\n",
			       nextstep, f, (bytes_tx[f]*8)/(timestep*1000),
			       (bytes_rx[f]*8)/(timestep*1000), avg_delay);

			bytes_tx[f] = 0;
			tx[f] = 0;
			bytes_rx[f] = 0;
			delay[f] = 0;
			rx[f] = 0;
		}

		nextstep = int($2/timestep)*timestep + timestep;
	}

	if($1 == "+" && $3 == "0") {
		if($8 > flows)
			flows = $8;
		time_buf[$12] = $2;
	}

	if($1 == "-" && $3 == "0") {
		bytes_tx[$8] += $6;
		tx[$8]++;
	}

	if($1 == "r" && $4 == "4") {
		bytes_rx[$8] += $6;
		delay[$8] += $2 - time_buf[$12];
		rx[$8]++;
	}

}
