package main

import (
	"expvar"
	"flag"
	"io"
	"log"
	"math/rand"
	"net/http"
	"runtime"
	"time"

	"github.com/yvasiyarov/gorelic"
)

var newrelicLicense = flag.String("newrelic-license", "", "Newrelic license")

var numCalls = expvar.NewInt("num_calls")

type WaveMetrica struct {
	sawtoothMax     int
	sawtoothCounter int
}

func (metrica *WaveMetrica) GetName() string {
	return "Custom/Wave_Metrica"
}
func (metrica *WaveMetrica) GetUnits() string {
	return "Queries/Second"
}
func (metrica *WaveMetrica) GetValue() (float64, error) {
	metrica.sawtoothCounter++
	if metrica.sawtoothCounter > metrica.sawtoothMax {
		metrica.sawtoothCounter = 0
	}
	return float64(metrica.sawtoothCounter), nil
}

func allocateAndSum(arraySize int) int {
	arr := make([]int, arraySize, arraySize)
	for i := range arr {
		arr[i] = rand.Int()
	}
	time.Sleep(time.Duration(rand.Intn(3000)) * time.Millisecond)

	result := 0
	for _, v := range arr {
		result += v
	}
	//log.Printf("Array size is: %d, sum is: %d\n", arraySize, result)
	return result
}

func doSomeJob(numRoutines int) {
	for i := 0; i < numRoutines; i++ {
		go allocateAndSum(rand.Intn(1024) * 1024)
	}
	log.Printf("All %d routines started\n", numRoutines)
	time.Sleep(1000 * time.Millisecond)
	runtime.GC()
}

func helloServer(w http.ResponseWriter, req *http.Request) {
	doSomeJob(5)
	io.WriteString(w, "Did some work")
}

func main() {
	flag.Parse()
	if *newrelicLicense == "" {
		log.Fatalf("Please, pass a valid newrelic license key.\n Use --help to get more information about available options\n")
	}
	agent := gorelic.NewAgent()
	agent.Verbose = true
	agent.CollectHTTPStat = true
	agent.NewrelicLicense = *newrelicLicense
	agent.AddCustomMetric(&WaveMetrica{
		sawtoothMax:     10,
		sawtoothCounter: 5,
	})
	agent.Run()

	http.HandleFunc("/", agent.WrapHTTPHandlerFunc(helloServer))
	http.ListenAndServe(":8080", nil)
}
