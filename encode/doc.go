package encode

import (
	"fmt"
	"time"
)

//Debug flag
const Debug bool = false
const perf bool = true

//PerfLog logs for performance metrics
func PerfLog(s string) {
	if perf == true {
		fmt.Printf("perf. Current time: %s. Log: %s, \n", time.Now(), s)
	}
}
