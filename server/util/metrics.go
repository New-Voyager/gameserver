package util

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type metrics struct {
	newGameReceivedCounter   prometheus.Counter
	newHandMsgSentCounter    prometheus.Counter
	handEndMsgSentCounter    prometheus.Counter
	activeGamesMapCountGauge prometheus.Gauge
}

func (m *metrics) NewGameReceived() {
	m.newGameReceivedCounter.Inc()
}

func (m *metrics) NewHandMsgSent() {
	m.newHandMsgSentCounter.Inc()
}

func (m *metrics) HandEndMsgSent() {
	m.handEndMsgSentCounter.Inc()
}

func (m *metrics) SetActiveGamesMapCount(count int) {
	m.activeGamesMapCountGauge.Set(float64(count))
}

var Metrics = &metrics{
	newGameReceivedCounter: promauto.NewCounter(prometheus.CounterOpts{
		Name: "new_games_received_total",
		Help: "Total number of new games received",
	}),
	newHandMsgSentCounter: promauto.NewCounter(prometheus.CounterOpts{
		Name: "new_hand_msg_sent_total",
		Help: "Total number of NEW_HAND messages sent",
	}),
	handEndMsgSentCounter: promauto.NewCounter(prometheus.CounterOpts{
		Name: "hand_end_msg_sent_total",
		Help: "Total number of hand END messages sent",
	}),
	activeGamesMapCountGauge: promauto.NewGauge(prometheus.GaugeOpts{
		Name: "active_games_map_entries_count",
		Help: "Count of the entries in the nats game manager activeGames map",
	}),
}
