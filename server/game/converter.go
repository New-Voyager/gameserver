package game

import "voyager.com/server/util"

func (g *Game) convertToServerUnits(msgItem *HandMessageItem) error {
	var err error
	switch msgItem.MessageType {
	case HandPlayerActed:
		err = g.convertHandPlayerActed(msgItem)
	default:
	}
	return err
}

func (g *Game) convertToClientUnits(message *HandMessage) error {
	for _, msgItem := range message.GetMessages() {
		msgType := msgItem.GetMessageType()
		switch msgType {
		case HandNewHand:
			convertNewHand(msgItem)
		case HandQueryCurrentHand:
			convertQueryCurrentHand(msgItem)
		case HandNextAction:
			convertNextAction(msgItem)
		case HandPlayerAction:
			convertYourAction(msgItem)
		case HandFlop:
			convertFlop(msgItem)
		case HandTurn:
			convertTurn(msgItem)
		case HandRiver:
			convertRiver(msgItem)
		case HandRunItTwice:
			convertRunItTwice(msgItem)
		case HandNoMoreActions:
			convertNoMoreActions(msgItem)
		case HandResultMessage2:
			convertResult(msgItem)
		default:
		}
	}
	return nil
}

func convertNoMoreActions(msgItem *HandMessageItem) {
	a := msgItem.GetNoMoreActions()
	for _, s := range a.Pots {
		s.Pot = util.CentsToChips(s.Pot)
	}
}

func convertQueryCurrentHand(msgItem *HandMessageItem) {
	c := msgItem.GetCurrentHandState()
	c.BigBlind = util.CentsToChips(c.BigBlind)
	c.SmallBlind = util.CentsToChips(c.SmallBlind)
	c.Straddle = util.CentsToChips(c.Straddle)
	c.PotUpdates = util.CentsToChips(c.PotUpdates)
	c.BombPotBet = util.CentsToChips(c.BombPotBet)
	for _, p := range c.PlayersActed {
		p.Amount = util.CentsToChips(p.Amount)
		p.RaiseAmount = util.CentsToChips(p.RaiseAmount)
		p.BetAmount = util.CentsToChips(p.BetAmount)
	}
	for pid := range c.PlayersStack {
		c.PlayersStack[pid] = util.CentsToChips(c.PlayersStack[pid])
	}
	na := c.NextSeatAction
	convertNextSeatAction(na)
	for i := 0; i < len(c.Pots); i++ {
		c.Pots[i] = util.CentsToChips(c.Pots[i])
	}
}

func convertNewHand(msgItem *HandMessageItem) {
	h := msgItem.GetNewHand()
	h.SmallBlind = util.CentsToChips(h.SmallBlind)
	h.BigBlind = util.CentsToChips(h.BigBlind)
	h.BringIn = util.CentsToChips(h.BringIn)
	h.Straddle = util.CentsToChips(h.Straddle)
	h.BombPotBet = util.CentsToChips(h.BombPotBet)
	for _, p := range h.PlayersInSeats {
		p.Stack = util.CentsToChips(p.Stack)
		p.PlayerReceived = util.CentsToChips(p.PlayerReceived)
	}
	for _, p := range h.PlayersActed {
		p.Amount = util.CentsToChips(p.Amount)
		p.RaiseAmount = util.CentsToChips(p.RaiseAmount)
		p.BetAmount = util.CentsToChips(p.BetAmount)
	}
}

func convertYourAction(msgItem *HandMessageItem) {
	a := msgItem.GetSeatAction()
	convertNextSeatAction(a)
}

func convertNextSeatAction(a *NextSeatAction) {
	a.StraddleAmount = util.CentsToChips(a.StraddleAmount)
	a.CallAmount = util.CentsToChips(a.CallAmount)
	a.RaiseAmount = util.CentsToChips(a.RaiseAmount)
	a.MinBetAmount = util.CentsToChips(a.MinBetAmount)
	a.MaxBetAmount = util.CentsToChips(a.MaxBetAmount)
	a.MinRaiseAmount = util.CentsToChips(a.MinRaiseAmount)
	a.MaxRaiseAmount = util.CentsToChips(a.MaxRaiseAmount)
	a.AllInAmount = util.CentsToChips(a.AllInAmount)
	for _, bo := range a.BetOptions {
		bo.Amount = util.CentsToChips(bo.Amount)
	}
}

func convertNextAction(msgItem *HandMessageItem) {
	a := msgItem.GetActionChange()
	a.BetAmount = util.CentsToChips(a.BetAmount)
	a.PotUpdates = util.CentsToChips(a.PotUpdates)
	for i := 0; i < len(a.Pots); i++ {
		a.Pots[i] = util.CentsToChips(a.Pots[i])
	}
	for _, s := range a.SeatsPots {
		s.Pot = util.CentsToChips(s.Pot)
	}
}

func convertFlop(msgItem *HandMessageItem) {
	f := msgItem.GetFlop()
	for i := 0; i < len(f.Pots); i++ {
		f.Pots[i] = util.CentsToChips(f.Pots[i])
	}
	for _, s := range f.SeatsPots {
		s.Pot = util.CentsToChips(s.Pot)
	}
	for seatNo := range f.PlayerBalance {
		f.PlayerBalance[seatNo] = util.CentsToChips(f.PlayerBalance[seatNo])
	}
}

func convertTurn(msgItem *HandMessageItem) {
	f := msgItem.GetTurn()
	for i := 0; i < len(f.Pots); i++ {
		f.Pots[i] = util.CentsToChips(f.Pots[i])
	}
	for _, s := range f.SeatsPots {
		s.Pot = util.CentsToChips(s.Pot)
	}
	for seatNo := range f.PlayerBalance {
		f.PlayerBalance[seatNo] = util.CentsToChips(f.PlayerBalance[seatNo])
	}
}

func convertRiver(msgItem *HandMessageItem) {
	f := msgItem.GetRiver()
	for i := 0; i < len(f.Pots); i++ {
		f.Pots[i] = util.CentsToChips(f.Pots[i])
	}
	for _, s := range f.SeatsPots {
		s.Pot = util.CentsToChips(s.Pot)
	}
	for seatNo := range f.PlayerBalance {
		f.PlayerBalance[seatNo] = util.CentsToChips(f.PlayerBalance[seatNo])
	}
}

func convertRunItTwice(msgItem *HandMessageItem) {
	r := msgItem.GetRunItTwice()
	for _, s := range r.SeatsPots {
		s.Pot = util.CentsToChips(s.Pot)
	}
}

func convertResult(msgItem *HandMessageItem) {
	handResultClient := msgItem.GetHandResultClient()
	for _, w := range handResultClient.PotWinners {
		w.Amount = util.CentsToChips(w.Amount)
		for _, wi := range w.BoardWinners {
			wi.Amount = util.CentsToChips(wi.Amount)
			for _, hw := range wi.HiWinners {
				hw.Amount = util.CentsToChips(hw.Amount)
			}
			for _, lw := range wi.LowWinners {
				lw.Amount = util.CentsToChips(lw.Amount)
			}
		}
	}

	for _, pi := range handResultClient.PlayerInfo {
		pi.Balance.Before = util.CentsToChips(pi.Balance.Before)
		pi.Balance.After = util.CentsToChips(pi.Balance.After)
		pi.Received = util.CentsToChips(pi.Received)
		pi.RakePaid = util.CentsToChips(pi.RakePaid)
		pi.PotContribution = util.CentsToChips(pi.PotContribution)
	}

	handResultClient.TipsCollected = util.CentsToChips(handResultClient.TipsCollected)
}
