package main

import (
	"github.com/btittelbach/go-bbhw"

	"bytes"
	"container/ring"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"sort"
	"strings"
	"time"
)

// PWM library functions:
// SetPolarity(p bool)
// SetPWMFreqDuty(freq_hz, fraction float64)
// GetPWMFreqDuty() (freq_hz, fraction float64)
// SetDuty(fraction float64)
// SetPWM(period, duty time.Duration)
// GetPWM() (period, duty time.Duration)
// DisablePWM()

const (
	slotsPath = "/sys/devices/bone_capemgr.9/slots"
	pwmPath   = "/sys/devices/ocp.3/pwm_test_P9_14.16/"
	ainPath   = "/sys/devices/ocp.3/44e0d000.tscadc/tiadc/iio:device0/in_voltage4_raw"
	// Using sysfs for PWM control
	pwmDTO        = "am33xx_pwm"
	pwmP9_14      = "bone_pwm_P9_14"
	pwmP9_16      = "bone_pwm_P9_16"
	pwmP9_21      = "bone_pwm_P9_21"
	pwmP9_22      = "bone_pwm_P9_22"
	pwmPeriod     = 500000 * time.Nanosecond
	pwmResolution = 10 // smallest detectable unit
	// Though analog input starts at zero, lowest value to trigger lights is 30.
	// Values below 30 are dead zone on potentiometers, so we pad the bottom values.
	ainLevels       = 4096 // 0 - 4095
	ainMinPad       = 25
	clockDividerMin = 1
	clockDividerMax = 65534
	sampleAvgMin    = 1
	// Limit overall maximum current draw to 1.5 amps.
	// 500K ns * 1.5 amps / 0.7 amps =
	maxLEDCurrent   = 700  // enforced by resistors on light fixture
	maxTotalCurrent = 1400 // previously determined to not overheat fixture
	maxTotalDuty    = pwmPeriod * maxTotalCurrent / maxLEDCurrent
)

// translate command line options to ADC constants
var sampleAvgMap = map[int]byte{
	1:  ADC_AVG_1,
	2:  ADC_AVG_2,
	4:  ADC_AVG_4,
	8:  ADC_AVG_8,
	16: ADC_AVG_16,
}

// flags
var (
	debug      = flag.Bool("debug", false, "log debug messages")
	sleep      = flag.String("sleep", "0ms", "duration (string) between updates (default 0ms)")
	windowSize = flag.Int("window", 100, "size of averaging window (default 100)")
	// program clock divider to actual value - 1, i.e., default register value 0
	clockDivider = flag.Int("divider", clockDividerMin, "ADC clock divider (default 1; max 65534)")
	sampleAvg    = flag.Int("average", sampleAvgMin, "ADC sample averaging (default 1; possible values 1, 2, 4, 8, 16)")
)

func calcDuty(aout float64) time.Duration {
	// theoretical max is 500000 but avoid hitting
	// type Duration int64 as number of nanoseconds
	return time.Duration(math.Min(.03*math.Pow(aout, 2)+ainMinPad, 499990))
}

// calcMedian add aout to existing values to calculate median
func calcMedian(window *ring.Ring, aout int) float64 {
	var counts = make([]float64, 0, *windowSize)
	count := func(v interface{}) {
		counts = append(counts, v.(float64))
	}
	window.Value = float64(aout)
	window.Do(count)
	sort.Float64s(counts)
	return counts[*windowSize/2]
}

// set duty based on median calculation
func setDuty(pwm *bbhw.PWMLine, window *ring.Ring, aout int, step byte, duties *[]time.Duration, msgs *[]string) *ring.Ring {
	newAout := calcMedian(window, aout)
	newDuty := calcDuty(newAout)
	normalDuty := normalize(duties, newDuty)
	// we save raw values for normalization calcs but set pwm to normalized duty cycle
	if newDuty != (*duties)[step] {
		(*duties)[step] = newDuty
		pwm.SetPWM(pwmPeriod, normalDuty)
	}
	if *debug {
		(*msgs)[step] = fmt.Sprintf("%s   aout %4d  duty %9s", (*msgs)[step], int(newAout), normalDuty)
	}
	return window.Next()
}

func initWindow() *ring.Ring {
	w := ring.New(*windowSize)
	for i := 0; i < *windowSize; i++ {
		w.Value = float64(0)
		w = w.Next()
	}
	return w
}

func addDTOIfNotExists(dto string) {
	log.Println("looking for slots file")
	slotsFileName, err := bbhw.FindSlotsFile()
	if err != nil {
		log.Fatalln(err)
	}
	log.Println("found slots file", slotsFileName)
	time.Sleep(100 * time.Millisecond)
	log.Println("reading slots file")
	slots, err := ioutil.ReadFile(slotsFileName)
	if err != nil {
		log.Fatalln(err)
	}
	if bytes.Contains(slots, []byte(dto)) {
		log.Println("slots file already contains overlay", dto)
		return
	}
	log.Println("adding DTO", dto)
	if err := bbhw.AddDeviceTreeOverlay(dto); err != nil {
		log.Fatalln(err)
	}
	time.Sleep(100 * time.Millisecond)
}

func newPWM(pwmPin string) *bbhw.PWMLine {
	addDTOIfNotExists("bone_pwm_" + pwmPin)
	pwm, err := bbhw.NewBBBPWM(pwmPin)
	if err != nil {
		log.Fatalln(err)
	}
	pwm.SetPolarity(true)
	return pwm
}

type LED struct {
	pwm *bbhw.PWMLine
	win *ring.Ring
}

func initPWMs() map[byte]*LED {
	// do not remove pwm; will crash BBB
	addDTOIfNotExists(pwmDTO)
	pwm14 := newPWM("P9_14") // green
	pwm16 := newPWM("P9_21") // red
	pwm21 := newPWM("P9_16") // white
	pwm22 := newPWM("P9_22") // blue

	// map ADC step channels to PWM pins
	// adjusted LEDs to mirror RGBW on my potentiometer test board
	LEDMap := map[byte]*LED{
		0: &LED{pwm21, initWindow()},
		1: &LED{pwm14, initWindow()},
		2: &LED{pwm22, initWindow()},
		3: &LED{pwm16, initWindow()},
	}
	return LEDMap
}

func normalize(duties *[]time.Duration, duty time.Duration) time.Duration {
	var sum time.Duration
	for _, d := range *duties {
		sum += d
	}
	// only normalize if needed
	if sum > maxTotalDuty {
		return maxTotalDuty * duty / sum
	}
	return duty
}

func main() {
	var sleepDuration time.Duration
	var err error
	flag.Parse()
	if sleepDuration, err = time.ParseDuration(*sleep); err != nil {
		log.Fatal("could not interpret sleep duration '%v'", *sleep)
	}
	if (*clockDivider < clockDividerMin) || (*clockDivider > clockDividerMax) {
		log.Fatalf("illegal ADC clock divider: must be %v to %v", clockDividerMin, clockDividerMax)
	}

	LEDMap := initPWMs()

	ADCInit(byte(*clockDivider-1), sampleAvgMap[*sampleAvg])
	defer ADCDisable()

	// setup a data structure to map steps to pins and pwms
	// windows to average the analog input values
	var led *LED
	// for efficiency, though it seems to make no difference to cpu%
	duties := make([]time.Duration, 4)
	// for debug logging
	msgs := make([]string, 4) // 4 LED colors max

	for {
		if sleepDuration > 0 {
			time.Sleep(sleepDuration)
		}

		for step, aout := range ReadAnalog(P9_37, P9_38, P9_39, P9_40) {
			if *debug {
				msgs[step] = fmt.Sprintf("Step %d:  aout %4d", step, aout)
			}
			led = LEDMap[step]
			led.win = setDuty(led.pwm, led.win, aout, step, &duties, &msgs)
		}
		if *debug {
			fmt.Println(strings.Join(msgs, "   "))
		}
	}
}
