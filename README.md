A system for controlling mult-colored LEDs in a fixture using the BeagleBone Black
and the Go language. 

Here's the finished BeagleBone Black and cape in its case:
![Ready to install](/images/bbb_finished.jpg)

The useful bits in this directory are 

The Go source code:

 - LEDLightFantastic.go
 - adc.go

A shell script to cross-compile the Go code for the ARM processor:

 - gobbb.sh

An edited version of the /etc/rc.local file to start the light controller code on system
startup and disable the obnoxious heatbeat LED on the BeagleBone:

 - rc.local

All this is done as root, which by default has no password on the BeagleBone.
