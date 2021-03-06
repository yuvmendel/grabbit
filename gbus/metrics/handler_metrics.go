package metrics

import (
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_model/go"
	"github.com/sirupsen/logrus"
	"sync"
)

var (
	handlerMetricsByHandlerName = &sync.Map{}
)

const (
	failure       = "failure"
	success       = "success"
	handlerResult = "result"
	handlers      = "handlers"
	grabbitPrefix = "grabbit"
)

type HandlerMetrics struct {
	result  *prometheus.CounterVec
	latency prometheus.Summary
}

func AddHandlerMetrics(handlerName string) {
	handlerMetrics := newHandlerMetrics(handlerName)
	_, exists := handlerMetricsByHandlerName.LoadOrStore(handlerName, handlerMetrics)

	if !exists {
		prometheus.MustRegister(handlerMetrics.latency, handlerMetrics.result)
	}
}

func RunHandlerWithMetric(handleMessage func() error, handlerName string, logger logrus.FieldLogger) error {
	handlerMetrics := GetHandlerMetrics(handlerName)
	defer func() {
		if p := recover(); p != nil {
			if handlerMetrics != nil {
				handlerMetrics.result.WithLabelValues(failure).Inc()
			}

			panic(p)
		}
	}()

	if handlerMetrics == nil {
		logger.WithField("handler", handlerName).Warn("Running with metrics - couldn't find metrics for the given handler")
		return handleMessage()
	}

	err := trackTime(handleMessage, handlerMetrics.latency)

	if err != nil {
		handlerMetrics.result.WithLabelValues(failure).Inc()
	} else {
		handlerMetrics.result.WithLabelValues(success).Inc()
	}

	return err
}

func GetHandlerMetrics(handlerName string) *HandlerMetrics {
	entry, ok := handlerMetricsByHandlerName.Load(handlerName)
	if ok {
		return entry.(*HandlerMetrics)
	}

	return nil
}

func newHandlerMetrics(handlerName string) *HandlerMetrics {
	return &HandlerMetrics{
		result: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: grabbitPrefix,
				Subsystem: handlers,
				Name:      fmt.Sprintf("%s_result", handlerName),
				Help:      fmt.Sprintf("The %s's result", handlerName),
			},
			[]string{handlerResult}),
		latency: prometheus.NewSummary(
			prometheus.SummaryOpts{
				Namespace: grabbitPrefix,
				Subsystem: handlers,
				Name:      fmt.Sprintf("%s_latency", handlerName),
				Help:      fmt.Sprintf("The %s's latency", handlerName),
			}),
	}
}

func trackTime(functionToTrack func() error, observer prometheus.Observer) error {
	timer := prometheus.NewTimer(observer)
	defer timer.ObserveDuration()

	return functionToTrack()
}

func (hm *HandlerMetrics) GetSuccessCount() (float64, error) {
	return hm.getLabeledCounterValue(success)
}

func (hm *HandlerMetrics) GetFailureCount() (float64, error) {
	return hm.getLabeledCounterValue(failure)
}

func (hm *HandlerMetrics) GetLatencySampleCount() (*uint64, error) {
	m := &io_prometheus_client.Metric{}
	err := hm.latency.Write(m)
	if err != nil {
		return nil, err
	}

	return m.GetSummary().SampleCount, nil
}

func (hm *HandlerMetrics) getLabeledCounterValue(label string) (float64, error) {
	m := &io_prometheus_client.Metric{}
	err := hm.result.WithLabelValues(label).Write(m)

	if err != nil {
		return 0, err
	}

	return m.GetCounter().GetValue(), nil
}
