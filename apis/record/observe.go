package record

import (
	"MOSS_backend/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var inferSuccessCounter = promauto.NewCounter(prometheus.CounterOpts{
	Name: prometheus.BuildFQName(config.AppName, "infer", "success"),
})

var inferFailureCounter = promauto.NewCounter(prometheus.CounterOpts{
	Name: prometheus.BuildFQName(config.AppName, "infer", "failure"),
})

var inferStatusCounter = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: prometheus.BuildFQName(config.AppName, "infer", "status"),
	},
	[]string{"status_code"},
)

var inferOnFlightCounter = promauto.NewGauge(prometheus.GaugeOpts{
	Name: prometheus.BuildFQName(config.AppName, "infer", "on_flight"),
})

var userInferRequestOnFlight = promauto.NewGauge(prometheus.GaugeOpts{
	Name: prometheus.BuildFQName(config.AppName, "user_infer_request", "on_flight"),
})
