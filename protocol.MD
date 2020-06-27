The game server holds multiple games. Each game is represented by a Game object. The player
is represented by a player object. The game uses duplex communication to run the game. All
the game rules/hand evalualtion, moving the actions are done by the Game object.

Management messages
Buy in request
1. Buy-in request (BUYIN), P->G
2. Buy-in request coming in (BUYIN_REQUESTED) P<-G (owner)
3. Approve buy-in request (APPROVE_BUYIN), P->G (owner) 
4. Buyin approved (BUYIN_APPROVED), G->P
 or
3. Deny buy-in request (DENY_BUYIN), P->G (owner) 
4. Buyin denied (DENY_APPROVED), G->P


Join waiting list
1. Add to the queue with buyin request (JOIN_WAITING_LIST), P->G
2. Join queue request coming in (WAITING_LIST_REQUEST), P<-G (owner)
3. Approve join waiting request (APPROVE_WAITING_LIST), P->G
4. Approved join waiting list (WAITING_LIST_APPROVED) P<-G
or
3. deny join waiting request (DENY_WAITING_LIST), P->G
4. denied join waiting list (WAITING_LIST_DENIED) P<-G

Seat change button requests (SEAT_CHANGE_BUTTON) P2G
1. Request seat change button (SEAT_CHANGE_REQUEST) P->G
2. Game returns next seat change button (SEAT_CHANGE_POS) G->P

An open seat 9 and no-one is on the waiting list
1. A new player request to sit on the seat  
2. Sit request with buy-in (WAITING_TO_SIT), P->G 
3. Request to sit (REQUEST_TO_SIT), G->P (owner)
4. Approve sitting request (APPROVE_SIT_REQUEST) P->G (owner)
5. Approved sit request (APPROVED_SIT_REQUEST) G->P
An existing player returns and sits in a open seat
1. Select open seat (SIT_AT_SEAT) P->G
2. Approved sit request (APPROVED_SIT_REQUEST) G->P

A player wants to take a break
When a user takes a break, he should be able to post a message (taking dump, to the loo, attending kid, be right back)
1. A player request to take a break (REQUEST_TAKE_BREAK) P->G
2. Approve break (APPROVE_BREAK)
If only one player is left, then the game will be paused.

A player leaves game
When a player leaves a game, he should be able to broadcase pre-edited message/audio to the group.
good night folks, good night ladies, later fuckers, good night degens
1. A player sends Leave request (LEAVE_GAME)
2. Acknowledge leave request (LEAVE_GAME_ACCEPTED)
The player will not leave the table yet. A button will show to cancel leave request.
3. Cancel leave request (CANCEL_LEAVE_REQUEST)
4. Acknowledge leave requst is cancelled.

Hand messages
GS -> Players
1. New Hand
2. ActionTaken (Player, Action, Amount)
3. Announce Winner
4. Game is paused (GAME_PAUSED)

GS -> Player
1. Your action, Possible actions: Fold, Check, Call, BET, Raise, Straddle

Player -> GS
1. Player Action (Action, amount)

Message structure:
We will use json for communication between GS and players. For internal state storage, we will 
use protobuf.


ver: version number of the protocol (major.minor)
direction: "P_2_G", "G_2_P", "G_2_A"
message_type: 
There is a streaming socket connection (websocket) is
held by the player objc


Chat texts:
Fold its personal
You think I am scared
You are bluffing
