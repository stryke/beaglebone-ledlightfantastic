A collection of hardware and software for controlling 4-color LEDs using the BeagleBone Black open-hardware computer and the Go open source programming language (Golang). 

This project started as negative space, created when I removed the wall heater. My idea was to build a shelf where the heater stood and buy a light for the smaller space formerly occupied by the vent. My neighbor, an engineer, had begun a project for his employer centered around a BeagleBone Black computer. He thought I should build my own light fixture and controller.  

That was version 1.0. Twirl a dial to adjust a color. Version 1.1 added an auto mode, entered by putting one dial to zero and the other three to full intensity. The off dial becomes a throttle of sorts, selecting one of 10 overall rates of change. Each of the three still control their respective color intensities. But now these are only baselines, around which each color varies. Auto mode also injects a bit of randomness into both the ranges of color intensity and the rates of change to those intensities.

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

The negative space, where once a wall heater lived. 
![Project inspiration](/images/hole_formerly_known_as_heater.jpg)

The six 4-color LED stars are visible as bright spots on the aluminum frame.
![Development](/images/bbb_development.jpg)

Final breadboard before commiting to solder.
![Beagle Bone ready to solder](/images/ready_to_solder.jpg)

After soldering up a cape.
![Beagle Bone soldered](/images/soldered.jpg)

Here's the finished BeagleBone Black and cape in its case:
![Beagle Bone ready to install](/images/bbb_finished.jpg)

The completed light fixture mounted in the wall. The LEDs face away from the viewer. The light reflects off a plywood sheet, painted white. The camera sees more color variation than does the eye. Below the light is the top part of the shelf. On top of the shelf is a small wireless bridge for remote access to the BeagleBone and the larger power supply for the LEDs. (I ultimately ran an Ethernet cable under the house to our router and removed the wireless unit.) To the left of the Nest thermostat are the four potentiometers: red, green, blue, white. To the right are a small button and a larger rocker switch. The button puts the BeagleBone to sleep and wakes it up; the switch cuts off power to the LEDs.
![Installed](/images/in_place.jpg)

The finished shelf and light fixture, mounted and painted.
![Trimmed and painted](/images/completed_unit.jpg)
