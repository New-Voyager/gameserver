package game

import (
	"google.golang.org/protobuf/proto"
	"voyager.com/server/util"
)

func (g *Game) convertToServerUnits(msgItem *HandMessageItem) error {
	var err error
	switch msgItem.MessageType {
	case HandPlayerActed:
		err = g.c2sHandPlayerActed(msgItem)
	default:
	}
	return err
}

func (g *Game) c2sHandPlayerActed(msgItem *HandMessageItem) error {
	pa := msgItem.GetPlayerActed()
	pa.Amount = util.ChipsToCents(pa.Amount)
	pa.Stack = util.ChipsToCents(pa.Stack)
	pa.PotUpdates = util.ChipsToCents(pa.PotUpdates)
	return nil
}

func (g *Game) convertToClientUnits(message *HandMessage, outMsg *HandMessage) error {
	ma, _ := proto.Marshal(message)
	proto.Unmarshal(ma, outMsg)

	for _, msgItem := range outMsg.GetMessages() {
		msgType := msgItem.GetMessageType()
		switch msgType {
		case HandNewHand:
			s2cNewHand(msgItem)
		case HandQueryCurrentHand:
			s2cQueryCurrentHand(msgItem)
		case HandPlayerActed:
			s2cPlayerActed(msgItem)
		case HandNextAction:
			s2cNextAction(msgItem)
		case HandYourAction:
			s2cYourAction(msgItem)
		case HandFlop:
			s2cFlop(msgItem)
		case HandTurn:
			s2cTurn(msgItem)
		case HandRiver:
			s2cRiver(msgItem)
		case HandRunItTwice:
			s2cRunItTwice(msgItem)
		case HandNoMoreActions:
			s2cNoMoreActions(msgItem)
		case HandResultMessage2:
			s2cResult(msgItem)
		default:
		}
	}
	return nil
}

func s2cNoMoreActions(msgItem *HandMessageItem) {
	a := msgItem.GetNoMoreActions()
	for _, s := range a.Pots {
		s.Pot = util.CentsToChips(s.Pot)
	}
}

func s2cPlayerActed(msgItem *HandMessageItem) {
	a := msgItem.GetPlayerActed()
	a.Amount = util.CentsToChips(a.Amount)
	a.Stack = util.CentsToChips(a.Stack)
	a.PotUpdates = util.CentsToChips(a.PotUpdates)
}

func s2cQueryCurrentHand(msgItem *HandMessageItem) {
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

func s2cNewHand(msgItem *HandMessageItem) {
	h := msgItem.GetNewHand()
	h.SmallBlind = util.CentsToChips(h.SmallBlind)
	h.BigBlind = util.CentsToChips(h.BigBlind)
	h.BringIn = util.CentsToChips(h.BringIn)
	h.Straddle = util.CentsToChips(h.Straddle)
	h.BombPotBet = util.CentsToChips(h.BombPotBet)
	h.Ante = util.CentsToChips(h.Ante)
	for _, p := range h.PlayersInSeats {
		p.Stack = util.CentsToChips(p.Stack)
		p.PlayerReceived = util.CentsToChips(p.PlayerReceived)
	}
	for _, p := range h.PlayersActed {
		p.Amount = util.CentsToChips(p.Amount)
		p.RaiseAmount = util.CentsToChips(p.RaiseAmount)
		p.BetAmount = util.CentsToChips(p.BetAmount)
	}
	h.PotUpdates = util.CentsToChips(h.PotUpdates)
	for i := 0; i < len(h.Pots); i++ {
		h.Pots[i] = util.CentsToChips(h.Pots[i])
	}
}

func s2cYourAction(msgItem *HandMessageItem) {
	a := msgItem.GetSeatAction()
	convertNextSeatAction(a)
}

func convertNextSeatAction(a *NextSeatAction) {
	if a == nil {
		return
	}
	a.StraddleAmount = util.CentsToChips(a.StraddleAmount)
	a.CallAmount = util.CentsToChips(a.CallAmount)
	a.RaiseAmount = util.CentsToChips(a.RaiseAmount)
	a.MinBetAmount = util.CentsToChips(a.MinBetAmount)
	a.MaxBetAmount = util.CentsToChips(a.MaxBetAmount)
	a.MinRaiseAmount = util.CentsToChips(a.MinRaiseAmount)
	a.MaxRaiseAmount = util.CentsToChips(a.MaxRaiseAmount)
	a.AllInAmount = util.CentsToChips(a.AllInAmount)
	a.SeatInSoFar = util.CentsToChips(a.SeatInSoFar)
	for _, bo := range a.BetOptions {
		bo.Amount = util.CentsToChips(bo.Amount)
	}
	a.PotAmount = util.CentsToChips(a.PotAmount)
}

func s2cNextAction(msgItem *HandMessageItem) {
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

func s2cFlop(msgItem *HandMessageItem) {
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
	f.PotUpdates = util.CentsToChips(f.PotUpdates)
}

func s2cTurn(msgItem *HandMessageItem) {
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
	f.PotUpdates = util.CentsToChips(f.PotUpdates)
}

func s2cRiver(msgItem *HandMessageItem) {
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
	f.PotUpdates = util.CentsToChips(f.PotUpdates)
}

func s2cRunItTwice(msgItem *HandMessageItem) {
	r := msgItem.GetRunItTwice()
	for _, s := range r.SeatsPots {
		s.Pot = util.CentsToChips(s.Pot)
	}
}

func s2cResult(msgItem *HandMessageItem) {
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
