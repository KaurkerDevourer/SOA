package main

import (
	"fmt"
	"time"
	"github.com/google/uuid"
	"math/rand"
)

type UsersInfo struct {
	data map[uuid.UUID]UserInfo
	totalUserCount int
}

type UserInfo struct {
	UniqueId uuid.UUID
	Nickname string
}

type Role uint8

const (
	Civilian Role = iota
	Sheriff
	Mafia
	Ghost
)

type PlayerState struct {
	user UserInfo
	role Role
	isAlive bool
	isBot bool
}

type GameState struct {
	players []PlayerState
	alivePlayersLeft int
	aliveMafiaLeft int
	activeGame bool
}

func (gs *GameState) Start() {
	rand.Seed(time.Now().UnixNano())
	perm := rand.Perm(len(gs.players))
	i := 0
	for ; i <= gs.aliveMafiaLeft; i++ {
		gs.players[perm[i]].role = Mafia
	}
	gs.players[perm[i]].role = Sheriff
	go gs.ProcessGame()
}

func (gs *GameState) Voting() []uint {
	votes := make([]uint, len(gs.players))

	return votes
}

func (gs *GameState) ProcessDay() bool {
	/*votes := */gs.Voting()
	return gs.aliveMafiaLeft == 0 || (gs.aliveMafiaLeft == gs.alivePlayersLeft - gs.aliveMafiaLeft)
}

func (gs *GameState) ProcessNight() {

}

func (gs *GameState) ProcessGame() {
	gs.activeGame = true
	for {
		end := gs.ProcessDay()
		if end {
			break
		}
		gs.ProcessNight()
	}
	if gs.aliveMafiaLeft == 0 {
		// Civilian wins!
	} else {
		// Mafia wins!
	}
	gs.activeGame = false
	gs.players = nil
}

type ServerInfo struct {
	games map[uuid.UUID]*GameState
	waitingGames []uuid.UUID
	users UsersInfo
}

var global ServerInfo

func CreateNewGame(total, mafia int) uuid.UUID {
	unique_id := uuid.New()
	game := new(GameState)
	game.players = make([]PlayerState, 0)
	global.games[unique_id] = game
	game.aliveMafiaLeft = mafia
	game.alivePlayersLeft = total
	global.waitingGames = append(global.waitingGames, unique_id)

	return unique_id
}

func GetStartPlayersState(user_id uuid.UUID, isBot bool) PlayerState {
	return PlayerState {
		user: global.users.data[user_id],
		role: Civilian,
		isAlive: true,
		isBot: isBot,
	}
}

const defaultMafiaAmount = 1
const defaultPlayersAmount = 4

func ConnectRandom(user_id uuid.UUID, isBot bool) error {
	var game_id uuid.UUID
	if len(global.waitingGames) == 0 {
		game_id = CreateNewGame(defaultPlayersAmount, defaultMafiaAmount)
	} else {
		game_id = global.waitingGames[0]
	}
	global.games[game_id].players = append(global.games[game_id].players, GetStartPlayersState(user_id, isBot))
	if len(global.games[game_id].players) == 4 {
		global.games[game_id].Start()
		global.waitingGames = global.waitingGames[1:]
	}
	return nil
}

func ConnectSession(user_id uuid.UUID, session_id uuid.UUID) error {
	return nil
}

func CreateUser(name string) uuid.UUID {
	unique_id := uuid.New()
	user_info := UserInfo{unique_id, name}
	global.users.data[unique_id] = user_info
	global.users.totalUserCount++
	return unique_id
}

func InitServerInfo() ServerInfo {
	users_info := UsersInfo{make(map[uuid.UUID]UserInfo), 0}
	return ServerInfo{make(map[uuid.UUID]*GameState), make([]uuid.UUID, 0), users_info}
}

func main() {
	global = InitServerInfo()
	id1 := CreateUser("aboba")
	id2 := CreateUser("flexer")

	fmt.Println(global, id1, id2)
}