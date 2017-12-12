package gorelic

import (
	metrics "github.com/yvasiyarov/go-metrics"
	"time"
)

type baseTimerMetrica struct {
	dataSource metrics.Timer
	name       string
	units      string
}

func (metrica *baseTimerMetrica) GetName() string {
	return metrica.name
}

func (metrica *baseTimerMetrica) GetUnits() string {
	return metrica.units
}

type timerRate1Metrica struct {
	*baseTimerMetrica
}

func (metrica *timerRate1Metrica) GetValue() (float64, error) {
	return metrica.dataSource.Rate1(), nil
}

type timerRateMeanMetrica struct {
	*baseTimerMetrica
}

func (metrica *timerRateMeanMetrica) GetValue() (float64, error) {
	return metrica.dataSource.RateMean(), nil
}

type timerMeanMetrica struct {
	*baseTimerMetrica
}

func (metrica *timerMeanMetrica) GetValue() (float64, error) {
	return metrica.dataSource.Mean() / float64(time.Millisecond), nil
}

type timerMinMetrica struct {
	*baseTimerMetrica
}

func (metrica *timerMinMetrica) GetValue() (float64, error) {
	return float64(metrica.dataSource.Min()) / float64(time.Millisecond), nil
}

type timerMaxMetrica struct {
	*baseTimerMetrica
}

func (metrica *timerMaxMetrica) GetValue() (float64, error) {
	return float64(metrica.dataSource.Max()) / float64(time.Millisecond), nil
}

type timerPercentile75Metrica struct {
	*baseTimerMetrica
}

func (metrica *timerPercentile75Metrica) GetValue() (float64, error) {
	return metrica.dataSource.Percentile(0.75) / float64(time.Millisecond), nil
}

type timerPercentile90Metrica struct {
	*baseTimerMetrica
}

func (metrica *timerPercentile90Metrica) GetValue() (float64, error) {
	return metrica.dataSource.Percentile(0.90) / float64(time.Millisecond), nil
}

type timerPercentile95Metrica struct {
	*baseTimerMetrica
}

func (metrica *timerPercentile95Metrica) GetValue() (float64, error) {
	return metrica.dataSource.Percentile(0.95) / float64(time.Millisecond), nil
}
