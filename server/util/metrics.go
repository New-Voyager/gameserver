package util

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type metrics struct {
	newGameReceivedCounter prometheus.Counter
	activeGamesGauge       prometheus.Gauge
}

func (m *metrics) NewGameReceived() {
	m.newGameReceivedCounter.Inc()
}

func (m *metrics) SetNumActiveGames(numActiveGames int) {
	m.activeGamesGauge.Set(float64(numActiveGames))
}

var Metrics = &metrics{
	newGameReceivedCounter: promauto.NewCounter(prometheus.CounterOpts{
		Name: "new_games_received_total",
		Help: "Total number of new games received",
	}),
	activeGamesGauge: promauto.NewGauge(prometheus.GaugeOpts{
		Name: "num_active_games",
		Help: "Number of games currently hosted",
	}),
}
