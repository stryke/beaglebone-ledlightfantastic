package main

import (
	"log"
	"os"
	"syscall"
	"time"
	"unsafe"
)

// is this the ADC? /proc/device-tree/ocp/tscadc@44e0d000
// ADC_TSC Registers: start: 0x44E0_D000; end: 0x44E0_EFFF; size: 8KB

// AM335x Memory Addresses
const (
	MMAP_OFFSET = 0x44C00000
	MMAP_SIZE   = 0x481AEFFF - MMAP_OFFSET // 0x35AEFFF or 56,291,327
	// Clock Module Memory Registers
	CM_WKUP                    = 0x44E00400
	CM_WKUP_ADC_TSC_CLKCTRL    = CM_WKUP + 0xBC
	CM_WKUP_MODULEMODE_ENABLE  = 0x02
	CM_WKUP_IDLEST_DISABLED    = 0x03 << 16 // 0x30000 or 196608
	CM_WKUP_IDLEST_DISABLED_GO = 0x03

	// Analog Digital Converter Memory Registers
	ADC_TSC = 0x44E0D000
	// CTRL operator code; by default no hardware interrupts enabled
	ADC_CTRL                         = ADC_TSC + 0x40
	CTRL_ENABLE                      = 0x01
	CTRL_DISABLE                     = 0x00
	CTRL_STEP_ID_TAG                 = 0x01 << 1 // store Step ID in FIFO with data
	ADC_STEPCONFIG_WRITE_PROTECT_OFF = 0x01 << 2

	// ADCRANGE operator code
	ADC_ADCRANGE       = ADC_TSC + 0x48
	ADCRANGE_MIN_RANGE = 0x000
	ADCRANGE_MAX_RANGE = 0xFFF // 4095

	ADC_CLKDIV = ADC_TSC + 0x4C
	//CLOCK_DIVIDER = 0xA0 // 160, 24MHz / 160 = 150KHz

	ADC_STEPENABLE = ADC_TSC + 0x54
	ADCSTEPCONFIG1 = ADC_TSC + 0x64
	ADCSTEPDELAY1  = ADC_TSC + 0x68
	ADCSTEPCONFIG2 = ADC_TSC + 0x6C
	ADCSTEPDELAY2  = ADC_TSC + 0x70
	ADCSTEPCONFIG3 = ADC_TSC + 0x74
	ADCSTEPDELAY3  = ADC_TSC + 0x78
	ADCSTEPCONFIG4 = ADC_TSC + 0x7C
	ADCSTEPDELAY4  = ADC_TSC + 0x80
	ADCSTEPCONFIG5 = ADC_TSC + 0x84
	ADCSTEPDELAY5  = ADC_TSC + 0x88
	ADCSTEPCONFIG6 = ADC_TSC + 0x8C
	ADCSTEPDELAY6  = ADC_TSC + 0x90
	ADCSTEPCONFIG7 = ADC_TSC + 0x94
	ADCSTEPDELAY7  = ADC_TSC + 0x98
	ADCSTEPCONFIG8 = ADC_TSC + 0x9C
	ADCSTEPDELAY8  = ADC_TSC + 0xA0

	// ADC built-in sample averaging
	ADC_AVG_1       = 0x00 // no averaging
	ADC_AVG_2       = 0x01 // average over 2 samples
	ADC_AVG_4       = 0x02
	ADC_AVG_8       = 0x03
	ADC_AVG_16      = 0x04
	ADC_OPENDELAY   = 0x00 // taken from Vegetable Avenger
	ADC_SAMPLEDELAY = 0x01 // taken from Vegetable Avenger

	// Each FIFO holds up to 128 analog output values in a circular array.
	// The act of reading the FIFO data register moves the FIFO to the next
	// entry. We cannot use the Go slice of bytes to read the FIFO
	// as it sees even a multi-byte re-slice as multiple reads.
	ADC_FIFO0COUNT      = ADC_TSC + 0xE4
	ADC_FIFO0THRESHOLD  = ADC_TSC + 0xE8
	ADC_FIFO0DATA       = ADC_TSC + 0x100
	ADC_FIFO_COUNT_MASK = 0x7F
	ADC_FIFO_STEP_MASK  = 0xF0000
	ADC_FIFO_MASK       = 0xFFF
)

type Pin struct {
	name    string // readable name of pin
	bank_id byte   // pin number within each bank, should be 0-31
	eeprom  byte   // position in eeprom
}

type mappedRegisters struct {
	file     *os.File
	register []byte
	fifo     *uint32
}

var (
	isMapped bool = false
	mapped   *mappedRegisters

	P9_33 = Pin{"AIN4", 4, 71}
	P9_35 = Pin{"AIN6", 6, 73}
	P9_36 = Pin{"AIN5", 5, 72}
	P9_37 = Pin{"AIN2", 2, 69}
	P9_38 = Pin{"AIN3", 3, 70}
	P9_39 = Pin{"AIN0", 0, 67}
	P9_40 = Pin{"AIN1", 1, 68}
)

func mmapInit() error {
	var err error
	if isMapped {
		return nil
	}

	mapped = new(mappedRegisters)

	//Now MemoryMap
	mapped.file, err = os.OpenFile("/dev/mem", os.O_RDWR, 0666)
	if err != nil {
		return err
	}

	// Mmap returns []byte.
	// Go wraps the system memory mapping pretty thoroughly. First it makes the
	// system call to mmap. It uses the returned pointer address to build a
	// struct matching the internals of a Go slice data structure and then
	// converts that struct into a slice of bytes using unsafe.Pointer.
	// Slice memory layout:
	// var sl = struct {
	// 	addr uintptr // the pointer address returned by system mmap
	// 	len  int
	// 	cap  int
	// }{addr, length, length}
	// Use unsafeto turn sl into a []byte.
	// b := *(*[]byte)(unsafe.Pointer(&sl))
	mapped.register, err = syscall.Mmap(int(mapped.file.Fd()), MMAP_OFFSET, MMAP_SIZE, syscall.PROT_WRITE|syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return err
	}

	// The downside to Go memory mapping is that we can access memory only one byte at a time.
	// This is fatal to access the FIFO register. The FIFO register uses internal
	// magic to detect a read and move to the next value so we must read all 32 bits at once.
	mapped.fifo = (*uint32)(unsafe.Pointer(&mapped.register[ADC_FIFO0DATA-MMAP_OFFSET]))

	isMapped = true
	return nil
}

func ADCInit(clockDivider, sampleAvg byte) {
	if err := mmapInit(); err != nil {
		log.Fatalf("unable to initialize memory map: %s", err)
	}

	mr := mapped.register

	// enable the ADC clock by setting bit 1 high
	mr[CM_WKUP_ADC_TSC_CLKCTRL-MMAP_OFFSET] |= CM_WKUP_MODULEMODE_ENABLE
	// wait for the enable to complete
	for (mr[CM_WKUP_ADC_TSC_CLKCTRL-MMAP_OFFSET] & CM_WKUP_MODULEMODE_ENABLE) == 0 {
		// waiting for adc clock module to initialize
	}

	// CTRL (40h):
	// pre-disable the ADC module; store Step ID in FIFO with data;
	mr[ADC_CTRL-MMAP_OFFSET] = CTRL_DISABLE | CTRL_STEP_ID_TAG | ADC_STEPCONFIG_WRITE_PROTECT_OFF
	// step down the ADC clock
	mr[ADC_CLKDIV-MMAP_OFFSET] = clockDivider

	// default: SW enabled, one-shot; no averaging
	// set averaging the same for all
	// assign an ADCSTEPCONFIG for each ain pin
	// set SEL_INP and SEL_INM for each STEPCONFIG per Vegetable Avenger
	// painful because SEL_INM bits are split across bytes 1 & 2
	mr[ADCSTEPCONFIG1-MMAP_OFFSET] = sampleAvg << 2
	mr[ADCSTEPCONFIG1-MMAP_OFFSET+2] = 0x00 | (0x00 << 3) // SEL_INM (bits 16-18) | SEL_INP (bits 19-22)
	mr[ADCSTEPCONFIG1-MMAP_OFFSET+1] = 0x00 << 7          // lowest bit of SEL_INM (bit 15)
	mr[ADCSTEPCONFIG2-MMAP_OFFSET] = sampleAvg << 2
	mr[ADCSTEPCONFIG2-MMAP_OFFSET+2] = 0x00 | (0x01 << 3)
	mr[ADCSTEPCONFIG2-MMAP_OFFSET+1] = 0x01 << 7
	mr[ADCSTEPCONFIG3-MMAP_OFFSET] = sampleAvg << 2
	mr[ADCSTEPCONFIG3-MMAP_OFFSET+2] = 0x01 | (0x02 << 3)
	mr[ADCSTEPCONFIG3-MMAP_OFFSET+1] = 0x00 << 7
	mr[ADCSTEPCONFIG4-MMAP_OFFSET] = sampleAvg << 2
	mr[ADCSTEPCONFIG4-MMAP_OFFSET+2] = 0x01 | (0x03 << 3)
	mr[ADCSTEPCONFIG4-MMAP_OFFSET+1] = 0x01 << 7
	//mr[ADCSTEPCONFIG5-MMAP_OFFSET] = sampleAvg << 2
	//mr[ADCSTEPCONFIG5-MMAP_OFFSET+2] = 0x02 | (0x04 << 3)
	//mr[ADCSTEPCONFIG5-MMAP_OFFSET+1] = 0x00 << 7
	//mr[ADCSTEPCONFIG6-MMAP_OFFSET] = sampleAvg << 2
	//mr[ADCSTEPCONFIG6-MMAP_OFFSET+2] = 0x02 | (0x05 << 3)
	//mr[ADCSTEPCONFIG6-MMAP_OFFSET+1] = 0x01 << 7
	//mr[ADCSTEPCONFIG7-MMAP_OFFSET] = sampleAvg << 2
	//mr[ADCSTEPCONFIG7-MMAP_OFFSET+2] = 0x06 << 3
	//mr[ADCSTEPCONFIG7-MMAP_OFFSET+2] = 0x03 | (0x06 << 3)
	//mr[ADCSTEPCONFIG7-MMAP_OFFSET+1] = 0x00 << 7
	// set sample delay as appropriate; veggie avenger uses 1
	mr[ADCSTEPDELAY1-MMAP_OFFSET+3] = ADC_SAMPLEDELAY
	mr[ADCSTEPDELAY2-MMAP_OFFSET+3] = ADC_SAMPLEDELAY
	mr[ADCSTEPDELAY3-MMAP_OFFSET+3] = ADC_SAMPLEDELAY
	mr[ADCSTEPDELAY4-MMAP_OFFSET+3] = ADC_SAMPLEDELAY
	//mr[ADCSTEPDELAY5-MMAP_OFFSET+3] = ADC_SAMPLEDELAY
	//mr[ADCSTEPDELAY6-MMAP_OFFSET+3] = ADC_SAMPLEDELAY
	//mr[ADCSTEPDELAY7-MMAP_OFFSET+3] = ADC_SAMPLEDELAY

	// restore write protection
	mr[ADC_CTRL-MMAP_OFFSET] &^= ADC_STEPCONFIG_WRITE_PROTECT_OFF
}

// ADCDisable shuts down the ADC and closes the memory mapping.
func ADCDisable() {
	mapped.register[ADC_CTRL-MMAP_OFFSET] = CTRL_DISABLE
	mapped.file.Close()
}

// ReadAnalog reads from one or more analog pins and returns
// a map of ADC step IDs to analog output values from 0-4095
func ReadAnalog(pins ...Pin) map[byte]int {
	if !isMapped {
		log.Fatalln("must initialize memory mapping")
	}

	if pins == nil {
		log.Fatalln("must read at least one pin")
	}

	var count byte
	for count = getFIFOCount(); count != 0; count = getFIFOCount() {
		log.Println("initial FIFO count should be zero: found", count)
		readFIFO(1)
		time.Sleep(850 * time.Microsecond)
	}

	// enable the step sequencer for this pin
	// no guarantee on output order when multiple pins are enabled
	enableStepSequencer(mapped.register, pins)
	time.Sleep(500 * time.Microsecond)

	aoutMap := readFIFO(len(pins))
	disableStepSequencer(mapped.register, pins)
	return aoutMap
}

func readFIFO(pinCt int) map[byte]int {
	aoutMap := make(map[byte]int, pinCt)
	var fifo uint32
	var step byte
	var aout int
	for count := getFIFOCount(); count > 0; count = getFIFOCount() {
		fifo = *mapped.fifo // read 32-bit FIFO register in one read
		step = byte((fifo & ADC_FIFO_STEP_MASK) >> 16)
		aout = int(fifo & ADC_FIFO_MASK)
		aoutMap[step] = aout
	}
	return aoutMap
}

func getFIFOCount() byte {
	return mapped.register[ADC_FIFO0COUNT-MMAP_OFFSET] & ADC_FIFO_COUNT_MASK
}

func enableStepSequencer(mr []byte, pins []Pin) {
	var bits byte = 0x00
	for _, pin := range pins {
		bits |= 0x01 << (pin.bank_id + 1)
	}
	mr[ADC_STEPENABLE-MMAP_OFFSET] |= bits
	// enable the ADC
	mr[ADC_CTRL-MMAP_OFFSET] |= CTRL_ENABLE
}

func disableStepSequencer(mr []byte, pins []Pin) {
	var bits byte = 0x00
	for _, pin := range pins {
		bits |= 0x01 << (pin.bank_id + 1)
	}
	mr[ADC_STEPENABLE-MMAP_OFFSET] &^= bits
	// disable the ADC
	mr[ADC_CTRL-MMAP_OFFSET] &^= CTRL_ENABLE
}
