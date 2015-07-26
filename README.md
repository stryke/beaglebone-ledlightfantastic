A system for controlling mult-colored LEDs in a fixture using the BeagleBone Black
and the Go language (Golang). 

The useful bits in this directory are 

The Go source code:

 - LEDLightFantastic.go
 - adc.go

A shell script to cross-compile the Go code for the ARM processor:

 - gobbb.sh

An edited version of the /etc/rc.local file to start the light controller code on system
startup and disable the bright heatbeat LED on the BeagleBone:

 - rc.local

All this is done as root, which by default has no password on the BeagleBone.

This project started out as negative space, created by the removal of a wall heater. 
![Project inspiration](/images/hole_formerly_known_as_heater.jpg)

My idea was to build a shelf where the heater was and buy a light for the smaller
space formerly occupied by the vent. My neighbor thought I should do something a bit more challenging...
![Development](/images/bbb_development.jpg)

Final breadboard before commiting to solder...
![Beagle Bone ready to solder](/images/ready_to_solder.jpg)

After soldering up a cape...
![Beagle Bone soldered](/images/soldered.jpg)

Here's the finished BeagleBone Black and cape in its case:
![Beagle Bone ready to install](/images/bbb_finished.jpg)
