package main

import (
	"math/rand"

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
	// auto mode
	// thresholds for OFF and ON
	aoutOff = 10
	aoutOn  = 4000
	// autoLoop controls when the aout offset is changed
	autoLoopMax    = 400 // we add min pad to get minpad to max+minpad
	autoLoopAdjust = 5   // frequency of change to auto loop max
	// autoOffset controls by how much aout is adjusted
	autoOffsetDelta  = 2
	autoOffsetMax    = 400 // outer bounds +/-
	autoOffsetAdjust = 5   // frequency of change to auto offset max
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

func calcDuty(aout float64) time.Duration {
	// theoretical max is 500000 but avoid hitting
	// type Duration int64 as number of nanoseconds
	return time.Duration(math.Min(.03*math.Pow(aout, 2)+ainMinPad, 499990))
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

// set duty based on median calculation
func setDuty(pwm *bbhw.PWMLine, aout float64, step byte, duties *[]time.Duration, msgs *[]string) {
	newDuty := calcDuty(aout)
	normalDuty := normalize(duties, newDuty)
	// we save raw values for normalization calcs but set pwm to normalized duty cycle
	if newDuty != (*duties)[step] {
		(*duties)[step] = newDuty
		pwm.SetPWM(pwmPeriod, normalDuty)
	}
	if *debug {
		(*msgs)[step] = fmt.Sprintf("%s   duty %9s", (*msgs)[step], normalDuty)
	}
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
	// auto mode
	autoLoop        int // current loop number
	autoLoopMax     int // number of loops between changes to aout offset
	autoOffset      int // offset to aout in auto mode
	autoOffsetDelta int // direction to change aout offset
	autoOffsetMax   int
}

// Incoming aout always reflects the current pot setting. What varies
// over time is the autoOffset, which starts out at zero and always
// remains within +/-autoOffsetMax.
func (led *LED) autoAdjust(aout int, loopMax int, updateLoopSize bool) {
	// increment/decrement the offset
	led.autoLoop++
	if led.autoLoop > led.autoLoopMax {
		led.autoOffset += led.autoOffsetDelta
		led.autoLoop = 0
		// Switch offset direction if led hit a boundary.  Boundaries includes
		// zero and the maximum possible level.  The two fixed boundaries
		// prevent an LED from parking at one intensity.
		if led.autoOffset >= led.autoOffsetMax || led.autoOffset <= -led.autoOffsetMax || (aout+led.autoOffset) <= aoutOff || (aout+led.autoOffset) >= aoutOn {
			led.autoOffsetDelta = -led.autoOffsetDelta
			// every so often change max size of offset
			// esp. important for fast changing settings
			if rand.Intn(autoOffsetAdjust) == 0 {
				led.autoOffsetMax = randomAutoOffsetMax(autoOffsetMax)
			}
		}
		// every so often change size of auto loop to change the change
		if updateLoopSize || rand.Intn(autoLoopAdjust) == 0 {
			led.autoLoopMax = randomAutoLoopMax(loopMax)
		}
	}
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
		0: &LED{
			pwm:             pwm21,
			win:             initWindow(),
			autoLoopMax:     randomAutoLoopMax(autoLoopMax),
			autoOffsetDelta: randomAutoOffsetDelta(),
			autoOffsetMax:   randomAutoOffsetMax(autoOffsetMax),
		},
		1: &LED{
			pwm:             pwm14,
			win:             initWindow(),
			autoLoopMax:     randomAutoLoopMax(autoLoopMax),
			autoOffsetDelta: randomAutoOffsetDelta(),
			autoOffsetMax:   randomAutoOffsetMax(autoOffsetMax),
		},
		2: &LED{
			pwm:             pwm22,
			win:             initWindow(),
			autoLoopMax:     randomAutoLoopMax(autoLoopMax),
			autoOffsetDelta: randomAutoOffsetDelta(),
			autoOffsetMax:   randomAutoOffsetMax(autoOffsetMax),
		},
		3: &LED{
			pwm:             pwm16,
			win:             initWindow(),
			autoLoopMax:     randomAutoLoopMax(autoLoopMax),
			autoOffsetDelta: randomAutoOffsetDelta(),
			autoOffsetMax:   randomAutoOffsetMax(autoOffsetMax),
		},
	}
	return LEDMap
}

func randomAutoOffsetMax(offsetMax int) int {
	// avoid panic
	if offsetMax < 1 {
		offsetMax = 1
	}
	// larger number means more variability
	const offsetRangeRatio int = 4
	offsetMinPad := offsetMax / offsetRangeRatio
	return rand.Intn(offsetMax-offsetMinPad) + offsetMinPad
}

func randomAutoLoopMax(loopMax int) int {
	// avoid panic
	if loopMax < 1 {
		loopMax = 1
	}
	// larger number means more variability
	const loopRangeRatio int = 4
	loopMinPad := loopMax / loopRangeRatio
	return rand.Intn(loopMax-loopMinPad) + loopMinPad
}

func randomAutoOffsetDelta() int {
	if rand.Intn(2) == 0 {
		return autoOffsetDelta
	}
	return -autoOffsetDelta
}

// translate pot aout to auto loop max size
func calcStepLoopMax(aout float64) int {
	switch {
	case aout < 20:
		return 1024 // lowest speed
	case aout < 60:
		return 512
	case aout < 130:
		return 256
	case aout < 200:
		return 128
	case aout < 400:
		return 64
	case aout < 800:
		return 32
	case aout < 1200:
		return 16
	case aout < 2000:
		return 8
	case aout < 3500:
		return 4
	case aout < 4090:
		return 2
	}
	return 1 // highest speed
}

// calcAutoMode sets autoMode to true if one pot is off and three are on,
// false if all pots are off, and returns the input value otherwise.
// Also calculated and returned is the step number that was set to off.
// The off step is used to set the maximum loop speed.
func calcAutoMode(autoMode bool, autoLoopStep byte, aoutMap map[byte]int) (bool, byte) {
	var offCt, onCt uint8
	var ls byte
	for step, aout := range aoutMap {
		switch {
		case aout < aoutOff:
			offCt += 1
			ls = step // iff auto mode switches on
		case aout > aoutOn:
			onCt += 1
		}
	}
	if offCt == 4 {
		return false, autoLoopStep // set auto mode off
	}
	if offCt == 1 && onCt == 3 {
		return true, ls // set auto mode on
	}
	return autoMode, autoLoopStep // leaves as is
}

func main() {
	rand.Seed(time.Now().UnixNano())
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

	var aoutMap map[byte]int
	var medAout float64                 // median value of aout
	var autoMode bool                   // auto mode continuously varies light intensity
	var autoLoopStep, prevLoopStep byte // pot that affects loop size, i.e., variation speed
	var stepLoopMax int                 // maximum loop size setting
	var updateLoopSize bool             // triggers immediate recalc of loop size
	for {
		if sleepDuration > 0 {
			time.Sleep(sleepDuration)
		}

		aoutMap = ReadAnalog(P9_37, P9_38, P9_39, P9_40)
		autoMode, autoLoopStep = calcAutoMode(autoMode, autoLoopStep, aoutMap)
		for step, aout := range aoutMap {
			led = LEDMap[step]
			medAout = calcMedian(led.win, aout)
			led.win = led.win.Next()

			if autoMode && step == autoLoopStep {
				updateLoopSize = autoLoopStep != prevLoopStep
				stepLoopMax = calcStepLoopMax(medAout)
				prevLoopStep = autoLoopStep
				if *debug {
					msgs[step] = fmt.Sprintf("STEP %d:  aout %6.1f  loop max %4d", step, medAout, stepLoopMax)
				}
				continue
			}

			if autoMode && medAout > aoutOff {
				led.autoAdjust(int(medAout), stepLoopMax, updateLoopSize)
				medAout += float64(led.autoOffset)
				if medAout < 0 {
					medAout = 0
				}
			}
			if *debug {
				msgs[step] = fmt.Sprintf("STEP %d:  aout %4d   aout %6.1f", step, aout, medAout)
			}
			setDuty(led.pwm, medAout, step, &duties, &msgs)
		}
		if *debug {
			fmt.Println(strings.Join(msgs, "     "))
		}
	}
}
