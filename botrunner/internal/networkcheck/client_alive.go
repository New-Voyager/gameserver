package networkcheck

import (
	"time"

	"github.com/rs/zerolog"
)

type ClientAliveCheck struct {
	logger    *zerolog.Logger
	logPrefix string
	gameID    uint64
	gameCode  string
	inAction  bool

	chEnd      chan bool
	chInAction chan bool

	sendAliveMsg func()
}

func NewClientAliveCheck(logger *zerolog.Logger, logPrefix string, gameID uint64, gameCode string, sendAliveMsg func()) *ClientAliveCheck {
	c := ClientAliveCheck{
		logger:       logger,
		gameID:       gameID,
		gameCode:     gameCode,
		chEnd:        make(chan bool, 10),
		chInAction:   make(chan bool, 10),
		sendAliveMsg: sendAliveMsg,
	}
	return &c
}

func (c *ClientAliveCheck) Run() {
	go c.loop()
}

func (c *ClientAliveCheck) Destroy() {
	c.chEnd <- true
}

func (c *ClientAliveCheck) InAction() {
	c.chInAction <- true
}

func (c *ClientAliveCheck) NotInAction() {
	c.chInAction <- false
}

func (c *ClientAliveCheck) loop() {
	ticker := time.NewTicker(2 * time.Second)

	for {
		select {
		case <-c.chEnd:
			return
		case isInAction := <-c.chInAction:
			c.logger.Info().Msgf("[%s] INACTION: %v", c.logPrefix, isInAction)
			c.inAction = isInAction
			if isInAction {
				// Immediately send one without waiting for the tick.
				c.sendAliveMsg()
			}
		case <-ticker.C:
			if c.inAction {
				c.sendAliveMsg()
			}
		}
	}
}
