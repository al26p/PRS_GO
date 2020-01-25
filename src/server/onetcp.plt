# set the terminal, i.e., the figure format (eps) and font (Helvetica, 20pt)
set term postscript eps enhanced "Helvetica" 20 

# reset all options to default, just for precaution
reset

# set the figure size
set size 0.7,0.7

##############
# throughput #
##############

# set the figure name
set output "onetcp_tput.eps"

# set the x axis
set xrange [0:100]
set xlabel "Time (s)"
set xtics 0,20,100
set mxtics 2

# set the y axis
set yrange [0:2500]
set ylabel "Throughput (kbps)"
set ytics 0,500,2500
set mytics 2

# set the legend (boxed, on the bottom)
set key box left width 1 height 0.5 samplen 2

# set the grid (grid lines start from tics on both x and y axis)
set grid xtics ytics

# plot the data from the log file
plot "< awk '$2 == 0 {print}' onetcp_postprocess.log" u 1:4 t "TCP 0" \
     w l lt 1 lw 3 lc rgb "#cc0000"

#########
# delay #
#########

# set the figure name
set output "onetcp_delay.eps"

# set the x axis
set xrange [0:100]
set xlabel "Time (s)"
set xtics 0,20,100
set mxtics 2

# set the y axis
set yrange [0:500]
set ylabel "Delay (ms)"
set ytics 0,100,500
set mytics 2

# set the legend (boxed, on the bottom)
set key box left width 1 height 0.5 samplen 2

# set the grid (grid lines start from tics on both x and y axis)
set grid xtics ytics

# plot the data from the log file
plot "< awk '$2 == 0 {print}' onetcp_postprocess.log" u 1:($5*1000) t "TCP 0" \
     w l lt 1 lw 3 lc rgb "#cc0000"


#############
# window 15 #
#############
set output "onetcp_wnd_15.eps"

set xrange [0:15]
set xlabel "Time (s)"
set xtics 0,1,15
set mxtics 1

set yrange [0:160]
set ylabel "Windows Size"
set ytics 0,20,160
set mytics 2

# set the legend (boxed, on the bottom)
set key box left width 1 height 0.5 samplen 2

# set the grid (grid lines start from tics on both x and y axis)
set grid xtics ytics

plot "< awk '$2 == 0 {print}' onetcp_wnd.tr" u 1:3 t "TCP 0" \
     w l lt 1 lw 3 lc rgb "#cc0000"

#############
# window 15 #
#############
set output "onetcp_wnd_full.eps"

set xrange [0:100]
set xlabel "Time (s)"
set xtics 0,20,100
set mxtics 2

set yrange [0:160]
set ylabel "Windows Size"
set ytics 0,20,160
set mytics 2

# set the legend (boxed, on the bottom)
set key box left width 1 height 0.5 samplen 2

# set the grid (grid lines start from tics on both x and y axis)
set grid xtics ytics

plot "< awk '$2 == 0 {print}' onetcp_wnd.tr" u 1:3 t "TCP 0" \
     w l lt 1 lw 3 lc rgb "#cc0000"

