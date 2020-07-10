How to handle all in?
As soon as a player goes all in, we are going to track allin on each pot.
mainpot = 6

player1 goes all in 50
mainpot = 54
allin: true
players[1, 2, 3]
pot1 = 0
players: []

player2 calls 50
mainpot = 102
players[1, 2, 3]
allin: true
equity[50, 50, 2]

player3 calls 50
mainpot = 150
players[1, 2, 3]
equity[50, 50, 50]
pot1 := 0
players: []

flop
player2 goes all in for 50
pot1 := 50
pot1.allin: true
players: [2]

player3 calls 50
pot1 := 100
pot1.allin: true
players: [2, 3]

Scenario 2
player1 stack 50
player2 stack 75
player3 stack 100

player1 goes allin with 50
pot1 = 50
pot1.allin: true
pot1.players = [1]
pot2 = 50
pot2.players = []

betAmount = 75
if currentPot.allin && current
player2 goes allin with 75
pot2 = 200
pot1.allin: true
pot2 = 25

