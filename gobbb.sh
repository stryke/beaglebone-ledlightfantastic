#!/bin/bash -e

gopath=/Users/doug
#host=beaglebone.local
host=10.0.0.26

GOPATH=${gopath} GOARM=7 GOARCH=arm GOOS=linux go build LEDLightFantastic.go adc.go
scp LEDLightFantastic root@${host}:/root/
