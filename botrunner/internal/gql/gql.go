package gql

import (
	"context"
	"fmt"
	"time"

	"github.com/machinebox/graphql"
	"voyager.com/botrunner/internal/game"
	"voyager.com/gamescript"
)

func NewGQLHelper(client *graphql.Client, timeoutSec uint32, authToken string) *GQLHelper {
	return &GQLHelper{
		client:     client,
		timeoutSec: timeoutSec,
		authToken:  authToken,
	}
}

type GQLHelper struct {
	client     *graphql.Client
	timeoutSec uint32
	authToken  string
	IpAddress  string
}

func (g *GQLHelper) SetAuthToken(authToken string) {
	g.authToken = authToken
}

// CreatePlayer registers the player.
func (g *GQLHelper) CreatePlayer(name string, deviceID string, email string, password string, isBot bool) (string, error) {
	req := graphql.NewRequest(CreatePlayerGQL)
	req.Var("name", name)
	req.Var("deviceId", deviceID)
	req.Var("email", email)
	req.Var("password", password)
	req.Var("bot", isBot)
	req.Header.Set("Cache-Control", "no-cache")

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.timeoutSec)*time.Second)
	defer cancel()

	var respData CreatePlayerResp
	err := g.client.Run(ctx, req, &respData)
	if err != nil {
		return "", err
	}

	return respData.PlayerUUID, nil
}

// CreatePlayer registers the player.
func (g *GQLHelper) DealerChoice(gameCode string, gameType game.GameType) (bool, error) {
	req := graphql.NewRequest(DealerChoiceGQL)
	req.Var("gameCode", gameCode)
	req.Var("gameType", gameType.String())
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Authorization", g.authToken)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.timeoutSec)*time.Second)
	defer cancel()

	var respData struct {
		ret bool
	}

	err := g.client.Run(ctx, req, &respData)
	if err != nil {
		return false, err
	}

	return respData.ret, nil
}

// CreateClub creates a new club.
func (g *GQLHelper) CreateClub(name string, description string) (string, error) {
	req := graphql.NewRequest(CreateClubGQL)
	req.Var("name", name)
	req.Var("description", description)
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Authorization", g.authToken)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.timeoutSec)*time.Second)
	defer cancel()

	var respData CreateClubResp
	err := g.client.Run(ctx, req, &respData)
	if err != nil {
		return "", err
	}
	return respData.ClubCode, nil
}

// CreateClub creates a reward for a club.
func (g *GQLHelper) CreateClubReward(clubCode string, name string, rewardType string, scheduleType string, amount float32) (uint32, error) {
	req := graphql.NewRequest(ConfigureReward)

	req.Var("clubCode", clubCode)
	req.Var("name", name)
	req.Var("type", rewardType)
	req.Var("schedule", scheduleType)
	req.Var("amount", amount)
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Authorization", g.authToken)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.timeoutSec)*time.Second)
	defer cancel()

	var respData CreateRewardResp
	err := g.client.Run(ctx, req, &respData)
	if err != nil {
		return 0, err
	}
	return respData.RewardId, nil
}

// CreateClub creates a reward for a club.
func (g *GQLHelper) GetClubRewards(clubCode string) (*[]game.Reward, error) {
	req := graphql.NewRequest(GetRewards)

	req.Var("clubCode", clubCode)
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Authorization", g.authToken)
	type RewardsResp struct {
		Rewards []game.Reward
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.timeoutSec)*time.Second)
	defer cancel()

	var rewards RewardsResp
	err := g.client.Run(ctx, req, &rewards)
	if err != nil {
		return nil, err
	}
	return &rewards.Rewards, nil
}

// JoinClub makes a request to join the club (will need a separate approval).
func (g *GQLHelper) JoinClub(clubCode string) (string, error) {
	req := graphql.NewRequest(JoinClubGQL)
	req.Var("clubCode", clubCode)
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Authorization", g.authToken)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.timeoutSec)*time.Second)
	defer cancel()

	var respData JoinClubResp
	err := g.client.Run(ctx, req, &respData)
	if err != nil {
		return "", err
	}

	return respData.Status, nil
}

// GetClubID queries for the numeric club ID using the club code.
func (g *GQLHelper) GetClubID(clubCode string) (uint64, error) {
	req := graphql.NewRequest(ClubByIdGQL)
	req.Var("clubCode", clubCode)
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Authorization", g.authToken)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.timeoutSec)*time.Second)
	defer cancel()

	var respData ClubByIdResp
	err := g.client.Run(ctx, req, &respData)
	if err != nil {
		return 0, err
	}
	return respData.Club.ID, nil
}

// GetClubMembers queries for club member data.
func (g *GQLHelper) GetClubMembers(clubCode string) ([]ClubMemberEntry, error) {
	req := graphql.NewRequest(ClubMembersGQL)
	req.Var("clubCode", clubCode)
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Authorization", g.authToken)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.timeoutSec)*time.Second)
	defer cancel()

	var respData ClubMembersResp
	err := g.client.Run(ctx, req, &respData)
	if err != nil {
		return nil, err
	}
	return respData.ClubMembers, nil
}

// ApproveClubMember queries for club member data.
func (g *GQLHelper) ApproveClubMember(clubCode string, playerID string) (string, error) {
	req := graphql.NewRequest(ApproveMemberGQL)
	req.Var("clubCode", clubCode)
	req.Var("playerUuid", playerID)
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Authorization", g.authToken)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.timeoutSec)*time.Second)
	defer cancel()
	var respData ApproveMemberResp
	err := g.client.Run(ctx, req, &respData)
	if err != nil {
		return "", err
	}
	return respData.Status, nil
}

// GetClubMemberStatus queries for club member data.
func (g *GQLHelper) GetClubMemberStatus(clubCode string) (ClubInfoResp, error) {
	req := graphql.NewRequest(ClubMemberStatusGQL)
	req.Var("clubCode", clubCode)
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Authorization", g.authToken)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.timeoutSec)*time.Second)
	defer cancel()

	var respData struct {
		ClubInfo ClubInfoResp
	}
	err := g.client.Run(ctx, req, &respData)
	if err != nil {
		return respData.ClubInfo, err
	}
	return respData.ClubInfo, nil
}

// CreateGame creates a new game.
func (g *GQLHelper) CreateGame(clubCode string, opt game.GameCreateOpt) (string, error) {
	req := graphql.NewRequest(ConfigureGameGQL)
	req.Var("clubCode", clubCode)
	req.Var("title", opt.Title)
	req.Var("gameType", opt.GameType)
	req.Var("smallBlind", opt.SmallBlind)
	req.Var("bigBlind", opt.BigBlind)
	req.Var("utgStraddleAllowed", opt.UtgStraddleAllowed)
	req.Var("straddleBet", opt.StraddleBet)
	req.Var("minPlayers", opt.MinPlayers)
	req.Var("maxPlayers", opt.MaxPlayers)
	req.Var("gameLength", opt.GameLength)
	req.Var("buyInApproval", opt.BuyInApproval)
	req.Var("rakePercentage", opt.RakePercentage)
	req.Var("rakeCap", opt.RakeCap)
	req.Var("buyInMin", opt.BuyInMin)
	req.Var("buyInMax", opt.BuyInMax)
	req.Var("actionTime", opt.ActionTime)
	req.Var("rewardIds", opt.RewardIds)
	req.Var("runItTwiceAllowed", opt.RunItTwiceAllowed)
	req.Var("muckLosingHand", opt.MuckLosingHand)
	req.Var("roeGames", opt.RoeGames)
	req.Var("dealerChoiceGames", opt.DealerChoiceGames)
	req.Var("highHandTracked", opt.HighHandTracked)
	req.Var("appCoinsNeeded", opt.AppCoinsNeeded)
	req.Var("ipCheck", opt.IpCheck)
	req.Var("gpsCheck", opt.GpsCheck)

	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Authorization", g.authToken)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.timeoutSec)*time.Second)
	defer cancel()

	var respData ConfigureGameResp
	err := g.client.Run(ctx, req, &respData)
	if err != nil {
		return "", err
	}
	return respData.ConfiguredGame.GameCode, nil
}

// SitIn takes a seat in a game.
func (g *GQLHelper) SitIn(gameCode string, seatNo uint32, gps *gamescript.GpsLocation) (string, error) {
	req := graphql.NewRequest(JoinGameGQL)

	req.Var("gameCode", gameCode)
	req.Var("seatNo", seatNo)
	if gps != nil {
		gpsLoc := make(map[string]interface{})
		gpsLoc["lat"] = gps.Lat
		gpsLoc["long"] = gps.Long
		req.Var("gps", gpsLoc)
	}

	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Authorization", g.authToken)
	if g.IpAddress != "" {
		req.Header.Set("X-RealIP", g.IpAddress)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.timeoutSec)*time.Second)
	defer cancel()

	var respData JoinGameResp
	err := g.client.Run(ctx, req, &respData)
	if err != nil {
		return "", err
	}

	return respData.Status, nil
}

// BuyIn buys chips once seated in a game.
func (g *GQLHelper) BuyIn(gameCode string, amount float32) (BuyInResp, error) {
	req := graphql.NewRequest(BuyInGQL)

	req.Var("gameCode", gameCode)
	req.Var("amount", amount)
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Authorization", g.authToken)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.timeoutSec)*time.Second)
	defer cancel()

	var respData BuyInResp
	err := g.client.Run(ctx, req, &respData)
	if err != nil {
		return respData, err
	}

	return respData, nil
}

// LeaveGame leaves the game.
func (g *GQLHelper) LeaveGame(gameCode string) (bool, error) {
	req := graphql.NewRequest(LeaveGameGQL)

	req.Var("gameCode", gameCode)
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Authorization", g.authToken)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.timeoutSec)*time.Second)
	defer cancel()

	var respData LeaveGameResp
	err := g.client.Run(ctx, req, &respData)
	if err != nil {
		return false, err
	}

	return respData.Status, nil
}

// SwitchSeat switches a different seat
func (g *GQLHelper) SwitchSeat(gameCode string, toSeat int) (string, error) {
	req := graphql.NewRequest(SwitchSeatGQL)

	req.Var("gameCode", gameCode)
	req.Var("seatNo", toSeat)

	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Authorization", g.authToken)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.timeoutSec)*time.Second)
	defer cancel()

	var respData SwitchSeatResp
	err := g.client.Run(ctx, req, &respData)
	if err != nil {
		return "", err
	}

	return respData.Status, nil
}

// ReloadChips reloads chips
func (g *GQLHelper) ReloadChips(gameCode string, amount float32) (bool, error) {
	req := graphql.NewRequest(ReloadChipsGQL)

	req.Var("gameCode", gameCode)
	req.Var("amount", amount)

	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Authorization", g.authToken)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.timeoutSec)*time.Second)
	defer cancel()

	var respData ReloadChipsResp
	err := g.client.Run(ctx, req, &respData)
	if err != nil {
		return false, err
	}

	return respData.Approved, nil
}

// StartGame starts the game.
func (g *GQLHelper) StartGame(gameCode string) (string, error) {
	req := graphql.NewRequest(StartGameGQL)

	var respData StartGameResp
	req.Var("gameCode", gameCode)
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Authorization", g.authToken)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.timeoutSec)*time.Second)
	defer cancel()

	err := g.client.Run(ctx, req, &respData)
	if err != nil {
		return "", err
	}

	return respData.Status, nil
}

// EndGame marks the game to end after any ongoing hand.
func (g *GQLHelper) EndGame(gameCode string) (string, error) {
	req := graphql.NewRequest(EndGameGQL)

	var respData EndGameResp
	req.Var("gameCode", gameCode)
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Authorization", g.authToken)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.timeoutSec)*time.Second)
	defer cancel()

	err := g.client.Run(ctx, req, &respData)
	if err != nil {
		return "", err
	}

	return respData.Status, nil
}

// GetGameInfo queries the game info from the api server.
func (g *GQLHelper) GetGameInfo(gameCode string) (gameInfo game.GameInfo, err error) {
	req := graphql.NewRequest(GameInfoGQL)
	req.Var("gameCode", gameCode)
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Authorization", fmt.Sprintf("%s", g.authToken))

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.timeoutSec)*time.Second)
	defer cancel()

	var respData GameInfoResp
	err = g.client.Run(ctx, req, &respData)
	if err != nil {
		return
	}
	gameInfo = respData.GameInfo
	return
}

// GetGameID queries for the numeric game ID using the game code.
func (g *GQLHelper) GetGameID(gameCode string) (uint64, error) {
	req := graphql.NewRequest(GameByIdGQL)
	req.Var("gameCode", gameCode)
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Authorization", fmt.Sprintf("%s", g.authToken))

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.timeoutSec)*time.Second)
	defer cancel()

	var respData GameByIDResp
	err := g.client.Run(ctx, req, &respData)
	if err != nil {
		return 0, err
	}
	return respData.Game.ID, nil
}

// GetPlayerID queries for the numeric player ID based on the auth token.
func (g *GQLHelper) GetPlayerID() (PlayerID, error) {
	req := graphql.NewRequest(PlayerByIdGQL)
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Authorization", fmt.Sprintf("%s", g.authToken))

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.timeoutSec)*time.Second)
	defer cancel()

	var respData PlayerByIDResp
	err := g.client.Run(ctx, req, &respData)
	if err != nil {
		return PlayerID{}, err
	}
	return respData.Player, nil
}

// GetEncryptionKey queries for the player's encryption key.
func (g *GQLHelper) GetEncryptionKey() (string, error) {
	req := graphql.NewRequest(EncryptionKeyGQL)
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Authorization", fmt.Sprintf("%s", g.authToken))

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.timeoutSec)*time.Second)
	defer cancel()

	var respData EncryptionKeyResp
	err := g.client.Run(ctx, req, &respData)
	if err != nil {
		return "", err
	}
	return respData.EncryptionKey, nil
}

// ApproveClubMember queries for club member data.
func (g *GQLHelper) GetClubCode(clubName string) (string, error) {
	req := graphql.NewRequest(MyClubsGQL)
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Authorization", g.authToken)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.timeoutSec)*time.Second)
	defer cancel()
	var respData MyClubsResp
	err := g.client.Run(ctx, req, &respData)
	if err != nil {
		return "", err
	}

	for _, club := range respData.Clubs {
		if club.Name == clubName {
			return club.ClubCode, nil
		}
	}
	return "", nil
}

// RequestSeatChange requests for a seat change when available
func (g *GQLHelper) RequestSeatChange(gameCode string) (string, error) {
	req := graphql.NewRequest(SeatChangeRequestGQL)

	//var respData EndGameResp
	var requestTime SeatChangeRequestResp
	req.Var("gameCode", gameCode)
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Authorization", g.authToken)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.timeoutSec)*time.Second)
	defer cancel()

	err := g.client.Run(ctx, req, &requestTime)
	if err != nil {
		return "", err
	}

	return requestTime.Time, nil
}

// RequestSeatChange requests for a seat change when available
func (g *GQLHelper) RequestTakeBreak(gameCode string) (bool, error) {
	req := graphql.NewRequest(TakeBreakRequestGQL)

	//var respData EndGameResp
	var resp TakeBreakRequestResp
	req.Var("gameCode", gameCode)
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Authorization", g.authToken)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.timeoutSec)*time.Second)
	defer cancel()

	err := g.client.Run(ctx, req, &resp)
	if err != nil {
		return false, err
	}

	return resp.Status, nil
}

// RequestSitBack returning from break
func (g *GQLHelper) RequestSitBack(gameCode string, gps *gamescript.GpsLocation) (bool, error) {
	req := graphql.NewRequest(SitBackRequestGQL)

	//var respData EndGameResp
	var resp SitBackRequestResp
	req.Var("gameCode", gameCode)
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Authorization", g.authToken)
	if g.IpAddress != "" {
		req.Header.Set("X-RealIP", g.IpAddress)
	}
	if gps != nil {
		gpsLoc := make(map[string]interface{})
		gpsLoc["lat"] = gps.Lat
		gpsLoc["long"] = gps.Long
		req.Var("location", gpsLoc)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.timeoutSec)*time.Second)
	defer cancel()

	err := g.client.Run(ctx, req, &resp)
	if err != nil {
		return false, err
	}

	return resp.Status, nil
}

// ConfirmSeatChange confirms to make a seat change to a open seat
func (g *GQLHelper) ConfirmSeatChange(gameCode string, seatNo uint32) (bool, error) {
	req := graphql.NewRequest(ConfirmSeatChangeGQL)

	var confirm ConfirmSeatChangeResp
	req.Var("gameCode", gameCode)
	req.Var("seatNo", seatNo)
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Authorization", g.authToken)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.timeoutSec)*time.Second)
	defer cancel()

	err := g.client.Run(ctx, req, &confirm)
	if err != nil {
		return false, err
	}

	return confirm.Confirmed, nil
}

// DeclineSeatChange declines to make a seat change to a open seat
func (g *GQLHelper) DeclineSeatChange(gameCode string) (bool, error) {
	req := graphql.NewRequest(DeclineSeatChangeGQL)

	var decline DeclineSeatChangeResp
	req.Var("gameCode", gameCode)
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Authorization", g.authToken)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.timeoutSec)*time.Second)
	defer cancel()

	err := g.client.Run(ctx, req, &decline)
	if err != nil {
		return false, err
	}

	return decline.Declined, nil
}

// JoinWaitList allows a player to join waiting list of a game
func (g *GQLHelper) JoinWaitList(gameCode string) (bool, error) {
	req := graphql.NewRequest(JoinWaitListGQL)

	var confirm JoinWaitListResp
	req.Var("gameCode", gameCode)
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Authorization", g.authToken)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.timeoutSec)*time.Second)
	defer cancel()

	err := g.client.Run(ctx, req, &confirm)
	if err != nil {
		return false, err
	}

	return confirm.Confirmed, nil
}

// DeclineWaitListSeat allows a player to join waiting list of a game
func (g *GQLHelper) DeclineWaitListSeat(gameCode string) (bool, error) {
	req := graphql.NewRequest(DeclineWaitListGQL)

	var confirm DeclineWaitListResp
	req.Var("gameCode", gameCode)
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Authorization", g.authToken)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.timeoutSec)*time.Second)
	defer cancel()

	err := g.client.Run(ctx, req, &confirm)
	if err != nil {
		return false, err
	}

	return confirm.Confirmed, nil
}

// ResetDB resets database for debugging
func (g *GQLHelper) ResetDB() error {

	req := graphql.NewRequest(ResetDBGQL)

	req.Header.Set("Cache-Control", "no-cache")

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.timeoutSec)*time.Second)
	defer cancel()
	var resp struct {
		Reset bool `json:"reset"`
	}
	err := g.client.Run(ctx, req, &resp)
	if err != nil {
		fmt.Printf("Error: %+v", err)
		return err
	}

	return nil
}

// PauseGame pauses the game in next hand
func (g *GQLHelper) PauseGame(gameCode string) error {
	req := graphql.NewRequest(PauseGameGQL)

	req.Var("gameCode", gameCode)
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Authorization", g.authToken)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.timeoutSec)*time.Second)
	defer cancel()
	var resp interface{}
	err := g.client.Run(ctx, req, &resp)
	if err != nil {
		return err
	}

	return nil
}

// ResumeGame pauses the game in next hand
func (g *GQLHelper) ResumeGame(gameCode string) error {
	req := graphql.NewRequest(ResumeGameGQL)

	req.Var("gameCode", gameCode)
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Authorization", g.authToken)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.timeoutSec)*time.Second)
	defer cancel()
	var resp interface{}
	err := g.client.Run(ctx, req, &resp)
	if err != nil {
		return err
	}

	return nil
}

// HostRequestSeatChange initiates host seat change process
func (g *GQLHelper) HostRequestSeatChange(gameCode string) (bool, error) {
	req := graphql.NewRequest(HostSeatChangeRequestGQL)

	//var respData EndGameResp
	req.Var("gameCode", gameCode)
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Authorization", g.authToken)
	var resp struct {
		SeatChange bool `json:"seatChange"`
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.timeoutSec)*time.Second)
	defer cancel()

	err := g.client.Run(ctx, req, &resp)
	if err != nil {
		return false, err
	}

	return resp.SeatChange, nil
}

// HostRequestSeatChangeComplete completes the seat change process
func (g *GQLHelper) HostRequestSeatChangeComplete(gameCode string) (bool, error) {
	req := graphql.NewRequest(HostSeatChangeCompleteRequestGQL)

	//var respData EndGameResp
	req.Var("gameCode", gameCode)
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Authorization", g.authToken)
	var resp struct {
		SeatChange bool `json:"seatChange"`
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.timeoutSec)*time.Second)
	defer cancel()

	err := g.client.Run(ctx, req, &resp)
	if err != nil {
		return false, err
	}

	return resp.SeatChange, nil
}

// HostRequestSeatChangeSwap requests two swap seats between two players
func (g *GQLHelper) HostRequestSeatChangeSwap(gameCode string, seat1 uint32, seat2 uint32) (bool, error) {
	req := graphql.NewRequest(HostSeatChangeSwapGQL)

	//var respData EndGameResp
	req.Var("gameCode", gameCode)
	req.Var("seatNo1", seat1)
	req.Var("seatNo2", seat2)

	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Authorization", g.authToken)
	var resp struct {
		SeatChange bool `json:"seatChange"`
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.timeoutSec)*time.Second)
	defer cancel()

	err := g.client.Run(ctx, req, &resp)
	if err != nil {
		return false, err
	}

	return resp.SeatChange, nil
}

func (g *GQLHelper) UpdatePlayerGameConfig(gameCode string, runItTwiceAllowed *bool, muckLosingHand *bool) error {
	req := graphql.NewRequest(UpdatePlayerGameConfigGQL)
	type GameConfigChangeInput struct {
		RunItTwiceAllowed *bool `json:"runItTwicePrompt,omitempty"`
		MuckLosingHand    *bool `json:"muckLosingHand,omitempty"`
	}
	config := GameConfigChangeInput{
		RunItTwiceAllowed: runItTwiceAllowed,
		MuckLosingHand:    muckLosingHand,
	}
	//var respData EndGameResp
	req.Var("gameCode", gameCode)
	req.Var("config", config)

	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Authorization", g.authToken)
	var resp struct {
		Ret bool `json:"ret"`
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.timeoutSec)*time.Second)
	defer cancel()

	err := g.client.Run(ctx, req, &resp)
	if err != nil {
		fmt.Printf("err: %v", err)
		return err
	}

	return nil
}

func (g *GQLHelper) UpdateIpAddress(ipAddress string) error {
	req := graphql.NewRequest(IpChangedGQL)

	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Authorization", g.authToken)
	if g.IpAddress != "" {
		req.Header.Set("X-RealIP", g.IpAddress)
	}

	var resp struct {
		Ret bool `json:"ret"`
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.timeoutSec)*time.Second)
	defer cancel()

	err := g.client.Run(ctx, req, &resp)
	if err != nil {
		fmt.Printf("err: %v", err)
		return err
	}

	return nil
}

func (g *GQLHelper) UpdateGpsLocation(gps *gamescript.GpsLocation) error {
	req := graphql.NewRequest(GpsChangedGQL)

	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Authorization", g.authToken)
	if gps != nil {
		gpsLoc := make(map[string]interface{})
		gpsLoc["lat"] = gps.Lat
		gpsLoc["long"] = gps.Long
		req.Var("gps", gpsLoc)
	}

	var resp struct {
		Ret bool `json:"ret"`
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.timeoutSec)*time.Second)
	defer cancel()

	err := g.client.Run(ctx, req, &resp)
	if err != nil {
		fmt.Printf("err: %v", err)
		return err
	}

	return nil
}

// PostBlind posts blind in the game
func (g *GQLHelper) PostBlind(gameCode string) (bool, error) {
	req := graphql.NewRequest(PostBlindGQL)

	req.Var("gameCode", gameCode)
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Authorization", g.authToken)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.timeoutSec)*time.Second)
	defer cancel()
	type PostBlindResp struct {
		Status bool
	}

	var respData PostBlindResp
	err := g.client.Run(ctx, req, &respData)
	if err != nil {
		return respData.Status, err
	}

	return respData.Status, nil
}

// GameInfoGQL is the gql query string for gameinfo api.
const GameInfoGQL = `query game_info($gameCode: String!) {
    gameInfo(gameCode: $gameCode) {
        gameCode
        gameType
        title
        smallBlind
        bigBlind
        straddleBet
        utgStraddleAllowed
        buttonStraddleAllowed
        minPlayers
        maxPlayers
        gameLength
        buyInApproval
        breakLength
        autoKickAfterBreak
        waitlistAllowed
        sitInApproval
        maxWaitList
        rakePercentage
        rakeCap
        buyInMin
        buyInMax
        actionTime
        muckLosingHand
		runItTwiceAllowed
        waitForBigBlind
        startedBy
        startedAt
        endedBy
        endedAt
        template
        status
        tableStatus
        seatInfo {
            availableSeats
            playersInSeats {
                name
                playerId
                status
                playerUuid
                seatNo
                buyIn
                stack
				isBot
            }
        }
        gameToken
		playerGameStatus
		gameToPlayerChannel
		handToAllChannel
		playerToHandChannel
		handToPlayerChannel
		pingChannel
		pongChannel
    }
}`

type GameInfoResp struct {
	GameInfo game.GameInfo `json:"gameInfo"`
}

// CreatePlayerGQL is the mustation gql for registering a new user.
const CreatePlayerGQL = `mutation create_player(
	$name: String!,
	$deviceId: String!,
	$email: String!,
	$password: String!,
	$bot: Boolean
) {
	playerUUID: createPlayer(
		player: {
			name: $name
			deviceId: $deviceId
			email: $email
			password: $password
			isBot: $bot
		}
	)
}`

// CreatePlayerResp is the gql response for CreatePlayerGQL.
type CreatePlayerResp struct {
	PlayerUUID string
}

const CreateClubGQL = `mutation create_club($name: String!, $description: String!) {
	clubCode: createClub(
		club: {
			name: $name
			description: $description
		}
	)
}`

type CreateClubResp struct {
	ClubCode string
}

const JoinClubGQL = `mutation join_club($clubCode: String!) {
	status: joinClub(
		clubCode: $clubCode
	)
}`

type JoinClubResp struct {
	Status string
}

const ClubByIdGQL = `query get_club_id($clubCode: String!) {
	club: clubById(
		clubCode: $clubCode
	) {
		id
	}
}`

type ClubByIdResp struct {
	Club struct {
		ID uint64 `json:"id"`
	}
}

const ClubMembersGQL = `query get_members($clubCode: String!) {
	clubMembers(
		clubCode: $clubCode
	) {
		name
		playerId
		status
	}
}`

type ClubMemberEntry struct {
	Name     string
	PlayerID string `json:"playerId"`
	Status   string
}

type ClubMembersResp struct {
	ClubMembers []ClubMemberEntry
}

const ApproveMemberGQL = `mutation approve_member($clubCode: String!, $playerUuid: String!) {
	status: approveMember(
		clubCode: $clubCode
		playerUuid: $playerUuid
	)
}`

type ApproveMemberResp struct {
	Status string
}

const ClubMemberStatusGQL = `query what_is_my_member_status($clubCode: String!) {
	clubInfo(
		clubCode: $clubCode
	) {
		name
		status
	}
}`

type ClubInfoResp struct {
	Name   string
	Status string
}

const ConfigureGameGQL = `mutation configure_game(
	$clubCode: String!
	$title: String
	$gameType: GameType!
	$smallBlind: Float!
	$bigBlind: Float!
	$utgStraddleAllowed: Boolean
	$straddleBet: Float!
	$minPlayers: Int!
	$maxPlayers: Int!
	$gameLength: Int!
	$buyInApproval: Boolean
	$rakePercentage: Float
	$rakeCap: Float
	$buyInMin: Float!
	$buyInMax: Float!
	$actionTime: Int!
	$rewardIds: [Int!]
	$runItTwiceAllowed: Boolean
	$muckLosingHand: Boolean
	$roeGames: [GameType!]
	$dealerChoiceGames: [GameType!]
	$highHandTracked: Boolean
	$appCoinsNeeded: Boolean
	$ipCheck: Boolean
	$gpsCheck: Boolean
) {
	configuredGame: configureGame(
		clubCode: $clubCode
		game: {
			title: $title
			gameType: $gameType
			smallBlind: $smallBlind
			bigBlind: $bigBlind
			utgStraddleAllowed: $utgStraddleAllowed
			straddleBet: $straddleBet
			minPlayers: $minPlayers
			maxPlayers: $maxPlayers
			gameLength: $gameLength
			buyInApproval: $buyInApproval
			rakePercentage: $rakePercentage
			rakeCap: $rakeCap
			buyInMin: $buyInMin
			buyInMax: $buyInMax
			actionTime: $actionTime
			rewardIds: $rewardIds
			runItTwiceAllowed: $runItTwiceAllowed
			muckLosingHand: $muckLosingHand
			roeGames: $roeGames
			dealerChoiceGames: $dealerChoiceGames
			highHandTracked: $highHandTracked
			appCoinsNeeded: $appCoinsNeeded
			ipCheck: $ipCheck
			gpsCheck: $gpsCheck
		}
	) {
		gameCode
	}
}`

type ConfigureGameResp struct {
	ConfiguredGame struct {
		GameCode string
	}
}

const JoinGameGQL = `mutation join_game($gameCode: String!, $seatNo: Int!, $gps: LocationInput) {
	status: joinGame(
		gameCode: $gameCode
		seatNo: $seatNo
		location: $gps
	)
}`

type JoinGameResp struct {
	Status string
}

const GameByIdGQL = `query get_game_id($gameCode: String!) {
	game: gameById(
		gameCode: $gameCode
	) {
		id
	}
}`

type GameByIDResp struct {
	Game struct {
		ID uint64 `json:"id"`
	}
}

const PlayerByIdGQL = `query get_player_id {
	player: playerById {
		name
		id
	}
}`

type PlayerByIDResp struct {
	Player PlayerID
}

type PlayerID struct {
	Name string
	ID   uint64 `json:"id"`
}

const EncryptionKeyGQL = `query get_encryption_key {
	encryptionKey: encryptionKey
}`

type EncryptionKeyResp struct {
	EncryptionKey string
}

const BuyInGQL = `mutation buy_in($gameCode: String!, $amount: Float!) {
	status: buyIn(
		gameCode: $gameCode
		amount: $amount
	) {
		approved
		expireSeconds
	}
}`

type BuyInResp struct {
	Status struct {
		Approved      bool
		ExpireSeconds uint32
	}
}

const LeaveGameGQL = `mutation leave_game($gameCode: String!) {
	status: leaveGame(
		gameCode: $gameCode
	)
}`

type LeaveGameResp struct {
	Status bool
}

const SwitchSeatGQL = `mutation switch_seat($gameCode: String!, $seatNo: Int!) {
	status: switchSeat(
		gameCode: $gameCode
		seatNo: $seatNo
	)
}`

type ReloadChipsResp struct {
	Approved bool
}

const ReloadChipsGQL = `mutation reloadChips($gameCode: String!, $amount: Float!) {
	status: reload(
		gameCode: $gameCode
		amount: $amount
	) {
		approved
	}
}`

type SwitchSeatResp struct {
	Status string
}

const StartGameGQL = `mutation start_game($gameCode: String!) {
	status: startGame(
		gameCode: $gameCode
	)
}`

type StartGameResp struct {
	Status string
}

const EndGameGQL = `mutation end_game($gameCode: String!) {
	status: endGame(
		gameCode: $gameCode
	)
}`

type EndGameResp struct {
	Status string
}

const ConfigureReward = `mutation create_reward (
	$clubCode: String!
	$name: String!
	$type: RewardType!
	$amount: Float!
	$schedule: ScheduleType!) {
		rewardId: createReward(clubCode: $clubCode, 
			input: {
				name: $name
				type: $type
				schedule: $schedule
				amount: $amount 
			})
	}`

type CreateRewardResp struct {
	RewardId uint32 `json:"rewardId"`
}

const GetRewards = `query ($clubCode: String!) {
		rewards(clubCode: $clubCode) {
			id
			name
			amount
			schedule
		}
	}`

const SeatChangeRequestGQL = `mutation requestSeatChange($gameCode: String!) {
	time: requestSeatChange(
		gameCode: $gameCode
	)
}`

type SeatChangeRequestResp struct {
	Time string
}

const ConfirmSeatChangeGQL = `mutation confirmSeatChange($gameCode: String!, $seatNo: Int!) {
	confirmed: confirmSeatChange(
		gameCode: $gameCode
		seatNo: $seatNo
	)
}`

type ConfirmSeatChangeResp struct {
	Confirmed bool
}

const DeclineSeatChangeGQL = `mutation declineSeatChange($gameCode: String!) {
	declined: declineSeatChange(
		gameCode: $gameCode
	)
}`

type DeclineSeatChangeResp struct {
	Declined bool
}

const TakeBreakRequestGQL = `mutation takeBreak($gameCode: String!) {
	status: takeBreak(
		gameCode: $gameCode
	)
}`

type TakeBreakRequestResp struct {
	Status bool
}

const SitBackRequestGQL = `mutation sitBack($gameCode: String!, $location: LocationInput) {
	status: sitBack(
		gameCode: $gameCode
		location: $location
	)
}`

type SitBackRequestResp struct {
	Status bool
}

const JoinWaitListGQL = `mutation joinWaitList($gameCode: String!) {
	confirmed: addToWaitingList(
		gameCode: $gameCode
	)
}`

type JoinWaitListResp struct {
	Confirmed bool
}

const DeclineWaitListGQL = `mutation declineWaitlistSeat($gameCode: String!) {
	confirmed: declineWaitlistSeat(
		gameCode: $gameCode
	)
}`

const PauseGameGQL = `mutation pauseGame($gameCode: String!) {
	pauseGame(
		gameCode: $gameCode
	)
}`

const ResumeGameGQL = `mutation resumeGame($gameCode: String!) {
	resumeGame(
		gameCode: $gameCode
	)
}`

type DeclineWaitListResp struct {
	Confirmed bool
}

// CreatePlayerResp is the gql response for CreatePlayerGQL.
type Club struct {
	ClubCode    string
	Name        string
	MemberCount int32
	IsOwner     bool
	ClubStatus  string
}

type MyClubsResp struct {
	Clubs []Club
}

const MyClubsGQL = `query  {
	clubs: myClubs {
		clubCode
		name
		memberCount
		isOwner
		clubStatus		
	}
}`

const ResetDBGQL = `mutation {
	reset: resetDB
}`

const HostSeatChangeRequestGQL = `mutation beginHostSeatChange($gameCode: String!) {
	seatChange: beginHostSeatChange(
		gameCode: $gameCode
	)
}`

const HostSeatChangeCompleteRequestGQL = `mutation seatChangeComplete($gameCode: String!) {
	seatChange: seatChangeComplete(
		gameCode: $gameCode
	)
}`

const HostSeatChangeSwapGQL = `mutation seatChangeSwapSeats($gameCode: String!, $seatNo1: Int!, $seatNo2: Int!) {
	seatChange: seatChangeSwapSeats(
		gameCode: $gameCode
		seatNo1: $seatNo1
		seatNo2: $seatNo2
	)
}`

const UpdatePlayerGameConfigGQL = `
	mutation update_player_game_config($gameCode:String! $config:PlayerGameConfigChangeInput!) {
		ret: updatePlayerGameConfig(gameCode:$gameCode, config:$config)
  	}`

const IpChangedGQL = `
	mutation ipChanged {
		ret: ipChanged
  	}`

const GpsChangedGQL = `
	mutation updateLocation($gps: LocationInput!) {
		ret: updateLocation(location: $gps)
  	}`

const DealerChoiceGQL = `mutation dealerChoice($gameCode: String!, $gameType: GameType!) {
	ret: dealerChoice(
		gameCode: $gameCode
		gameType: $gameType
	)
}`

const PostBlindGQL = `mutation post_blind($gameCode: String!) {
	status: postBlind(
		gameCode: $gameCode
	)
}`
