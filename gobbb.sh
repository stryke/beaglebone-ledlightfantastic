#!/bin/bash -e

GOPATH=/Users/doug/Dropbox/go GOARM=7 GOARCH=arm GOOS=linux go build LEDLightFantastic.go adc.go
scp LEDLightFantastic root@beaglebone.local:/root/
