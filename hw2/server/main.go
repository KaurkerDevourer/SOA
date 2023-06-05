package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"time"
	"sync"

	"github.com/KaurkerDevourer/SOA/hw1/pkg/mafiapb"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"github.com/pkg/errors"
)

type UsersInfo struct {
	data map[uuid.UUID]UserInfo
	totalUserCount int
}

type UserInfo struct {
	UniqueId uuid.UUID
	Nickname string
}

func (user *UserInfo) ToProtobuf() *mafia.UserInfo {
	return &mafia.UserInfo {
		Username: user.Nickname,
		Id: user.UniqueId.String(),
	}
}

type Role uint8

const (
	Civilian Role = iota
	Sheriff
	Mafia
	Ghost
)

type GameStatus uint8

const (
	NotStarted GameStatus = iota
	Active
	Finished
)

var roleMap = map[Role]string {
	Civilian: "Civilian",
	Sheriff: "Sheriff",
	Mafia: "Mafia",
	Ghost: "Ghost",
}

type PlayerState struct {
	stream mafia.MafiaService_JoinGameServer
	user UserInfo
	role Role
	isAlive bool
}

type GameState struct {
	players []PlayerState
	alivePlayersLeft int
	aliveMafiaLeft int
	nightStalkersLeft int
	dayCount int
	status GameStatus
	wg sync.WaitGroup
	mutex sync.Mutex
	votes map[string][]string
	killedThisNight uuid.UUID
}

func (gs *GameState) Start(Id string) {
	log.Println("Starting game: ", Id)
	gs.mutex.Lock()
	rand.Seed(time.Now().UnixNano())
	perm := rand.Perm(len(gs.players))
	i := 0
	mafias := make([]*mafia.Vote, 0)
	for ; i < gs.aliveMafiaLeft; i++ {
		gs.players[perm[i]].role = Mafia
		mafias = append(mafias, &mafia.Vote{Who: gs.players[perm[i]].user.Nickname})
	}
	gs.players[perm[i]].role = Sheriff
	userInfos := make([]*mafia.UserInfo, 0)
	for _, state := range gs.players {
		userInfos = append(userInfos, state.user.ToProtobuf())
	}
	for _, state := range gs.players {
		votes := make([]*mafia.Vote, 0)
		if state.role == Mafia {
			votes = mafias
		} else {
			votes = nil
		}
		state.stream.Send(&mafia.WaitingGame{
			Type: mafia.EventType_GameStarted,
			Msg: roleMap[state.role],
			Players: userInfos,
			Count: int32(gs.aliveMafiaLeft),
			Id: Id,
			Votes: votes,
		})
	}
	gs.status = Active
	gs.mutex.Unlock()
	gs.ProcessGame()
}

func (gs *GameState) ValidateVote(request *mafia.VoteRequest, userId, kickId uuid.UUID) string {
	ok := false
	gs.mutex.Lock()
	for _, x := range gs.players {
		if x.user.UniqueId == userId && x.isAlive {
			ok = true
			break
		}
	}
	gs.mutex.Unlock()
	if (!ok) {
		return "You are dead or not in this game"
	}
	ok = false
	gs.mutex.Lock()
	for _, x := range gs.players {
		if x.user.UniqueId == kickId && x.isAlive {
			ok = true
			break
		}
	}
	gs.mutex.Unlock()
	if (!ok) {
		return "The player you want to kick is dead or not in this game"
	}
	return "OK"
}

func (gs *GameState) ProcessVote(request *mafia.VoteRequest) string {
	userId, _ := uuid.Parse(request.GetUserId())
	kickId, _ := uuid.Parse(request.GetKickUserId())
	err := gs.ValidateVote(request, userId, kickId)
	if (err != "OK") {
		return err
	}
	
	gs.mutex.Lock()
	_, ok := gs.votes[kickId.String()]
	if !ok {
		gs.votes[kickId.String()] = make([]string, 0, 1)
	}
	gs.votes[kickId.String()] = append(gs.votes[kickId.String()], userId.String())
	gs.mutex.Unlock()

	gs.wg.Done()

	return "OK"
}

func ConvertToProto(votes map[string][]string) []*mafia.Vote {
	ans := make([]*mafia.Vote, 0)
	for id, votes := range votes {
		ans = append(ans, &mafia.Vote{
			Who: id,
			ByWhome: votes,
		})
	}
	return ans
}

func (gs *GameState) ProcessDay() bool {
	gs.dayCount++
	if gs.dayCount == 1 {
		log.Println("First day processed")
		return false
	}
	gs.wg.Add(gs.alivePlayersLeft)
	gs.votes = map[string][]string{}
	for _, state := range gs.players {
		state.stream.Send(&mafia.WaitingGame{Type: mafia.EventType_ProcessDay})
	}
	gs.wg.Wait()
	absolute := false
	mx := 0
	var kicked string
	gs.mutex.Lock()
	for id, votes := range gs.votes {
		if len(votes) == mx {
			absolute = false
		}
		if len(votes) > mx {
			mx = len(votes)
			kicked = id
			absolute = true
		}
	}
	votes := ConvertToProto(gs.votes)
	if absolute {
		for _, state := range gs.players {
			if state.user.UniqueId.String() == kicked {
				state.isAlive = false
				gs.alivePlayersLeft--
				if state.role == Mafia {
					gs.aliveMafiaLeft--
					gs.nightStalkersLeft--
				} else if state.role == Sheriff {
					gs.nightStalkersLeft--
				}
				state.role = Ghost
				state.stream.Send(&mafia.WaitingGame{Type: mafia.EventType_VoteResults, Msg: "You got kicked", Votes: votes})
			} else {
				state.stream.Send(&mafia.WaitingGame{Type: mafia.EventType_VoteResults, Msg: "Player was kicked", Votes: votes, Id: kicked})
			}
		}
	} else {
		for _, state := range gs.players {
			state.stream.Send(&mafia.WaitingGame{Type: mafia.EventType_VoteResults, Msg: "Noone got kicked", Votes: votes})
		}
	}
	log.Println("Processed day ", gs.dayCount)
	ret := gs.aliveMafiaLeft == 0 || (gs.aliveMafiaLeft == gs.alivePlayersLeft - gs.aliveMafiaLeft) || gs.alivePlayersLeft == 3
	gs.mutex.Unlock()
	return ret
}

func (gs *GameState) ProcessNight() {
	gs.wg.Add(gs.nightStalkersLeft)
	log.Println("Process night", gs.dayCount)
	for _, state := range gs.players {
		state.stream.Send(&mafia.WaitingGame{Type: mafia.EventType_ProcessNight})
	}
	gs.wg.Wait()
	log.Println(gs.killedThisNight, " was killed this night.")
	for _, state := range gs.players {
		if state.user.UniqueId == gs.killedThisNight {
			state.isAlive = false
			gs.alivePlayersLeft--
			if state.role == Sheriff {
				gs.nightStalkersLeft--
			}
			state.role = Ghost
			state.stream.Send(&mafia.WaitingGame{Type: mafia.EventType_NightResults, Msg: "You got killed"})
		} else {
			state.stream.Send(&mafia.WaitingGame{Type: mafia.EventType_NightResults, Msg: "Player got killed", Id: gs.killedThisNight.String()})
		}
	}
}

func (gs *GameState) ProcessGame() {
	for {
		end := gs.ProcessDay()
		if end {
			break
		}
		gs.ProcessNight()
	}
	if gs.aliveMafiaLeft == 0 {
		log.Println("Civilian wins!")
		for _, state := range gs.players {
			state.stream.Send(&mafia.WaitingGame{Type: mafia.EventType_GameFinished, Msg: "Civilian wins!"})
		}
	} else {
		log.Println("Mafia wins!")
		for _, state := range gs.players {
			state.stream.Send(&mafia.WaitingGame{Type: mafia.EventType_GameFinished, Msg: "Mafia wins!"})
		}
	}
	gs.status = Finished
	gs.players = nil
}
 
type Server struct {
	mafia.UnimplementedMafiaServiceServer
	games map[uuid.UUID]*GameState
	waitingGames []uuid.UUID
	users UsersInfo
	mutex sync.Mutex
}

func (s *Server) Init() {
	s.users.data = make(map[uuid.UUID]UserInfo)
	s.games = make(map[uuid.UUID]*GameState)
	s.waitingGames = make([]uuid.UUID, 0)
}
 
func main() {
	log.Println("Server running ...")
 
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalln(err)
	}
 
	fmt.Print("Total players in one game: \n\n>>> ")
	_, err = fmt.Scanf("%d", &defaultPlayersAmount)
	fmt.Print("Total mafia in one game: \n\n>>> ")
	_, err = fmt.Scanf("%d", &defaultMafiaAmount)
	srv := grpc.NewServer()
	s := new(Server)
	s.Init()
	mafia.RegisterMafiaServiceServer(srv, s)
 
	log.Fatalln(srv.Serve(lis))
}


func (s *Server) CreateNewGame(total, mafia int) uuid.UUID {
	unique_id := uuid.New()
	game := new(GameState)
	game.players = make([]PlayerState, 0)
	s.games[unique_id] = game
	game.status = NotStarted
	game.aliveMafiaLeft = mafia
	game.alivePlayersLeft = total
	game.nightStalkersLeft = mafia + 1
	s.waitingGames = append(s.waitingGames, unique_id)

	return unique_id
}

func (s *Server) GetStartPlayersState(
	stream mafia.MafiaService_JoinGameServer,
	user_id uuid.UUID,
) PlayerState {
	return PlayerState {
		stream: stream,
		user: s.users.data[user_id],
		role: Civilian,
		isAlive: true,
	}
}

var defaultMafiaAmount = 1
var defaultPlayersAmount = 4
 
func (s *Server) CreateNewUser(ctx context.Context, request *mafia.CreateUser) (*mafia.UserInfo, error) {
	s.mutex.Lock()
	unique_id := uuid.New()
	username := request.GetUsername()
	log.Println("New user: ", username, "ID: ", unique_id)
	user_info := UserInfo{unique_id, username}
	s.users.data[unique_id] = user_info
	s.users.totalUserCount++
	log.Println("Total users: ", s.users.totalUserCount)

	s.mutex.Unlock()

	return &mafia.UserInfo{Id: unique_id.String(), Username: username}, nil
}

func (s *Server) NotifyNewPlayer(who []PlayerState, username string) {
	for _, state := range who {
		state.stream.Send(&mafia.WaitingGame{Type: mafia.EventType_PlayerJoined, Msg: username})
	}
}

func (s *Server) JoinGame(request *mafia.JoinMsg, stream mafia.MafiaService_JoinGameServer) error {
	var game_id uuid.UUID
	s.mutex.Lock()
	if len(s.waitingGames) == 0 {
		game_id = s.CreateNewGame(defaultPlayersAmount, defaultMafiaAmount)
	} else {
		game_id = s.waitingGames[0]
	}
	s.mutex.Unlock()
	userInfo := request.GetUserInfo()
	parsed, err := uuid.Parse(userInfo.GetId())
	if err != nil {
		return errors.Wrap(err, "failed to parse user id")
	}
	s.games[game_id].mutex.Lock()
	s.games[game_id].players = append(s.games[game_id].players, s.GetStartPlayersState(stream, parsed))
	s.NotifyNewPlayer(s.games[game_id].players[:len(s.games[game_id].players) - 1], userInfo.GetUsername())
	fl := len(s.games[game_id].players) == defaultPlayersAmount
	s.games[game_id].mutex.Unlock()
	if fl {
		s.games[game_id].mutex.Lock()
		s.waitingGames = s.waitingGames[1:]
		game := s.games[game_id]
		s.games[game_id].mutex.Unlock()
		game.Start(game_id.String())
	}
	log.Println("Game id:", game_id)

	userInfos := make([]*mafia.UserInfo, 0)
	for _, state := range s.games[game_id].players {
		userInfos = append(userInfos, state.user.ToProtobuf())
	}
	stream.Send(&mafia.WaitingGame{Type: mafia.EventType_EventWelcome, Msg: game_id.String(), Players: userInfos, Count: int32(s.games[game_id].aliveMafiaLeft)})
	for {
		s.games[game_id].mutex.Lock()
		fl := s.games[game_id].status == Finished
		s.games[game_id].mutex.Unlock()
		if fl {
			break
		}
		time.Sleep(time.Second)
	}
	return nil
}

func (s *Server) DayVote(ctx context.Context, request *mafia.VoteRequest) (*mafia.VoteResponse, error) {
	game_id, _ := uuid.Parse(request.GetGameId())

	game, ok := s.games[game_id]
	if (!ok) {
		return &mafia.VoteResponse{Ok: "No such game:" + game_id.String()}, nil
	}
	ans := game.ProcessVote(request)

	return &mafia.VoteResponse{Ok: ans}, nil
}

func (s *Server) NightVote(ctx context.Context, request *mafia.VoteRequest) (*mafia.VoteResponse, error) {
	game_id, _ := uuid.Parse(request.GetGameId())

	game, ok := s.games[game_id]
	if (!ok) {
		return &mafia.VoteResponse{Ok: "No such game:" + game_id.String()}, nil
	}
	userId, _ := uuid.Parse(request.GetUserId())
	kickId, _ := uuid.Parse(request.GetKickUserId())
	err := game.ValidateVote(request, userId, kickId)
	if err != "OK" {
		return &mafia.VoteResponse{Ok: err}, nil
	}
	for _, x := range game.players {
		if x.user.UniqueId == userId {
			if x.role == Mafia {
				game.killedThisNight = kickId
				break
			} else if x.role == Sheriff {
				for _, y := range game.players {
					if y.user.UniqueId == kickId {
						if y.role == Mafia {
							game.wg.Done()
							return &mafia.VoteResponse{Ok: "Your guess is right"}, nil
						} else {
							game.wg.Done()
							return &mafia.VoteResponse{Ok: "Your guess is wrong"}, nil
						}
					}
				}
			} else {
				return &mafia.VoteResponse{Ok: "You are sleeping at night, you are civilian"}, nil
			}
		}
	}
	game.wg.Done()
	return &mafia.VoteResponse{Ok: "OK"}, nil
}