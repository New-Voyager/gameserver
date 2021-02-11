package gql

import (
	"context"
	"fmt"
	"time"

	"github.com/machinebox/graphql"
	"voyager.com/botrunner/internal/game"
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
func (g *GQLHelper) SitIn(gameCode string, seatNo uint32) (string, error) {
	req := graphql.NewRequest(JoinGameGQL)

	req.Var("gameCode", gameCode)
	req.Var("seatNo", seatNo)
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Authorization", g.authToken)

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

// ConfirmSeatChange confirms to make a seat change to a open seat
func (g *GQLHelper) ConfirmSeatChange(gameCode string) (bool, error) {
	req := graphql.NewRequest(ConfirmSeatChangeGQL)

	var confirm ConfirmSeatChangeResp
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
        waitlistSupported
        sitInApproval
        maxWaitList
        rakePercentage
        rakeCap
        buyInMin
        buyInMax
        actionTime
        muckLosingHand
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
                playerUuid
                seatNo
                buyIn
                stack
            }
        }
        gameToken
		playerGameStatus
		gameToPlayerChannel
		handToAllChannel
		playerToHandChannel
		handToPlayerChannel
    }
}`

type GameInfoResp struct {
	GameInfo game.GameInfo
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

const JoinGameGQL = `mutation join_game($gameCode: String!, $seatNo: Int!) {
	status: joinGame(
		gameCode: $gameCode
		seatNo: $seatNo
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

const ConfirmSeatChangeGQL = `mutation confirmSeatChange($gameCode: String!) {
	confirmed: confirmSeatChange(
		gameCode: $gameCode
	)
}`

type ConfirmSeatChangeResp struct {
	Confirmed bool
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