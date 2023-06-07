package main
 
import (
	"context"
	"log"
	"time"
	"math/rand"
	"fmt"
	"bufio"
	"os"
	"io"
	"github.com/google/uuid"
	"github.com/KaurkerDevourer/SOA/hw3/pkg/mafiapb"
	"google.golang.org/grpc"
)

type Client struct {
	cli mafia.MafiaServiceClient
	stream mafia.MafiaService_JoinGameClient
	isBot bool
	ctx context.Context
}

type Role uint8

const (
	Civilian Role = iota
	Sheriff
	Mafia
	Ghost
)

var roleMap = map[string]Role {
	"Civilian": Civilian,
	"Sheriff": Sheriff,
	"Mafia": Mafia,
	"Ghost": Ghost,
}

type IsMafia uint8

const (
	Unknown IsMafia = iota
	No
	Yes
)

type UserInfo struct {
	UniqueId uuid.UUID
	Nickname string
	isMafia IsMafia
}

type GameState struct {
	role Role
	playersAlive []UserInfo
	game_id uuid.UUID
	user UserInfo
}

func GetPlayers(userInfos []*mafia.UserInfo) []UserInfo {
	users := make([]UserInfo, len(userInfos))
	for i, x := range userInfos {
		id := x.GetId()
		users[i].UniqueId, _ = uuid.Parse(id)
		users[i].Nickname = x.GetUsername()
		users[i].isMafia = Unknown
	}
	return users
} 

func ConvertToPrint(userInfos []*mafia.UserInfo) []string {
	str := make([]string, len(userInfos))
	for i, x := range userInfos {
		str[i] = x.GetUsername()
	}
	return str
}

func (client *Client) Vote(gs *GameState, scanner *bufio.Scanner, isDay, isSheriff bool) {
	if gs.role == Ghost {
		return
	}
	for {
		var kick_id string
		for {
			if client.isBot {
				rand.Seed(time.Now().UnixNano())
				perm := rand.Perm(len(gs.playersAlive))
				if isDay {
					if gs.role == Mafia {
						for j := 0; j < len(perm); j++ {
							if gs.playersAlive[perm[j]].isMafia == No {
								kick_id = gs.playersAlive[perm[j]].UniqueId.String()
								break
							}
						}
					} else {
						for j := 0; j < len(perm); j++ {
							if gs.playersAlive[perm[j]].isMafia == Yes {
								kick_id = gs.playersAlive[perm[j]].UniqueId.String()
								break
							}
						}
						if kick_id == "" {
							for j := 0; j < len(perm); j++ {
								if gs.playersAlive[perm[j]].UniqueId != gs.user.UniqueId && gs.playersAlive[perm[j]].isMafia == Unknown {
									kick_id = gs.playersAlive[perm[j]].UniqueId.String()
									break
								}
							}
						}
					}
				} else if isSheriff {
					for j := 0; j < len(perm); j++ {
						if gs.playersAlive[perm[j]].isMafia == Unknown {
							kick_id = gs.playersAlive[perm[j]].UniqueId.String()
							break
						}
					}
					if kick_id == "" {
						if gs.playersAlive[perm[0]].UniqueId == gs.user.UniqueId {
							kick_id = gs.playersAlive[perm[1]].UniqueId.String()
						} else {
							kick_id = gs.playersAlive[perm[0]].UniqueId.String()
						}
					}
				} else {
					for j := 0; j < len(perm); j++ {
						if gs.playersAlive[perm[j]].isMafia == No {
							kick_id = gs.playersAlive[perm[j]].UniqueId.String()
							break
						}
					}
				}
				break
			} else {
				if isDay {
					fmt.Print("Who you want to kick: \n\n>>> ")
				} else if isSheriff {
					fmt.Print("Who you want to check: \n\n>>> ")
				} else {
					fmt.Print("Who you want to kill: \n\n>>> ")
				}
				scanner.Scan()
				username := scanner.Text()
				end := false
				bad := false
				for _, x := range gs.playersAlive {
					if x.Nickname == username {
						if x.UniqueId == gs.user.UniqueId {
							if isDay {
								fmt.Println("You cant kick yourself")
							} else if isSheriff {
								fmt.Println("You cant check yourself")
							} else {
								fmt.Println("You cant kill yourself")
							}
							bad = true
						} else {
							end = true
							kick_id = x.UniqueId.String()
						}
					}
				}
				if end {
					break
				}
				flex := make([]string, len(gs.playersAlive))
				for i, x := range gs.playersAlive {
					flex[i] = x.Nickname
				}
				if !bad {
					fmt.Println("The player you want to kick is dead or not in this game:", username, " .Select one of", flex)
				}
			}
		}

		var kickname string
		for _, x := range gs.playersAlive {
			if x.UniqueId.String() == kick_id {
				kickname = x.Nickname
				break
			}
		}

		if isDay {
			fmt.Println("You are trying to kick ", kickname)
			response, err := client.cli.DayVote(client.ctx, &mafia.VoteRequest{
				GameId: gs.game_id.String(),
				UserId: gs.user.UniqueId.String(),
				KickUserId: kick_id,
			})
			if err == nil && response.GetOk() == "OK" {
				break
			}
			fmt.Println(response.GetOk())
		} else {
			if isSheriff {
				fmt.Println("You are trying to check ", kickname)
			} else {
				fmt.Println("You are trying to kill ", kickname)
			}
			response, err := client.cli.NightVote(client.ctx, &mafia.VoteRequest{
				GameId: gs.game_id.String(),
				UserId: gs.user.UniqueId.String(),
				KickUserId: kick_id,
			})
			if err == nil && (response.GetOk() == "Your guess is right" || response.GetOk() == "Your guess is wrong" || response.GetOk() == "OK") {
				if isSheriff && !isDay {
					log.Println(response.GetOk())
					if response.GetOk() == "Your guess is right" {
						for i, x := range gs.playersAlive {
							if x.UniqueId.String() == kick_id {
								gs.playersAlive[i].isMafia = Yes
							}
						}
					}

					if response.GetOk() == "Your guess is wrong" {
						for i, x := range gs.playersAlive {
							if x.UniqueId.String() == kick_id {
								gs.playersAlive[i].isMafia = No
							}
						}
					}
				}
				break
			}
			fmt.Println(response.GetOk())
		}
	}
}

func (gs *GameState) ConvertToNickname(id []string) []string {
	ans := make([]string, len(id))
	for i, x := range id {
		for _, y := range gs.playersAlive {
			if x == y.UniqueId.String() {
				ans[i] = y.Nickname
			}
		}
	}
	return ans
}

func (client *Client) Start(gs *GameState) {
	scanner := bufio.NewScanner(os.Stdin)
	for {
		event, err := client.stream.Recv()
		if err == io.EOF {
			break
	   	}

		if err != nil {
			log.Fatalf("\n\n ERROR ", err.Error(), "\n\n")
			break
		}

		switch event.GetType() {
		case mafia.EventType_ProcessDay:
			log.Println("Day processing")
			client.Vote(gs, scanner, true, true)
		case mafia.EventType_VoteResults:
			votes := event.GetVotes()
			for _, vote := range votes {
				byWhome := gs.ConvertToNickname(vote.ByWhome)
				who := gs.ConvertToNickname([]string{vote.Who})[0]
				log.Println(byWhome, " voted for ", who)
			}
			if event.GetMsg() == "You got kicked" {
				gs.role = Ghost
				log.Println(event.GetMsg())
				continue
			}
			if event.GetMsg() == "Noone got kicked" {
				log.Println(event.GetMsg())
				continue
			}
			dead, _ := uuid.Parse(event.GetId())
			for i, x := range gs.playersAlive {
				if x.UniqueId == dead {
					log.Println("Player " + x.Nickname + " was kicked")
					gs.playersAlive[len(gs.playersAlive) - 1], gs.playersAlive[i] = gs.playersAlive[i], gs.playersAlive[len(gs.playersAlive) - 1]
					gs.playersAlive = gs.playersAlive[:len(gs.playersAlive) - 1]
					break
				}
			}
		case mafia.EventType_ProcessNight:
			log.Println("Night processing")
			if gs.role == Mafia || gs.role == Sheriff {
				client.Vote(gs, scanner, false, gs.role == Sheriff)
			}
		
		case mafia.EventType_NightResults:
			if event.GetMsg() == "You got killed" {
				gs.role = Ghost
				log.Println(event.GetMsg())
				continue
			}

			dead, _ := uuid.Parse(event.GetId())
			for i, x := range gs.playersAlive {
				if x.UniqueId == dead {
					log.Println("Player " + x.Nickname + " was killed")
					gs.playersAlive[len(gs.playersAlive) - 1], gs.playersAlive[i] = gs.playersAlive[i], gs.playersAlive[len(gs.playersAlive) - 1]
					gs.playersAlive = gs.playersAlive[:len(gs.playersAlive) - 1]
					break
				}
			}
		case mafia.EventType_GameFinished:
			log.Println(event.GetMsg())
			return
		}
	}
}

func ConvertToList(mafias []*mafia.Vote) []string {
	ans := make([]string, len(mafias))
	for i, x := range mafias {
		ans[i] = x.Who
	}
	return ans
}

func (client *Client) playGame(response *mafia.UserInfo) {
	//isGameStarted := true
	//var role Role
	for {
		event, err := client.stream.Recv()
		if err == io.EOF {
		 	break
		}
		if err != nil {
			log.Fatalf("\n\n ERROR HERE ", err.Error(), "\n\n")
			break
		}
		switch event.GetType() {
		case mafia.EventType_EventWelcome:
			log.Println("You joined game ", event.GetId(), " Players list: ", ConvertToPrint(event.GetPlayers()))
		case mafia.EventType_PlayerJoined:
			log.Println("Player joined the game", event.GetMsg())
		case mafia.EventType_GameStarted:
			log.Println("Game", event.GetId(), " started. Your role is ", event.GetMsg(), "Players list: ", ConvertToPrint(event.GetPlayers()), "Mafia count: ", event.GetCount())
			if roleMap[event.GetMsg()] == Mafia {
				log.Println("Mafia list: ", ConvertToList(event.GetVotes()))
			}
			gs := new(GameState)
			gs.user = GetPlayers([]*mafia.UserInfo{response})[0]
			gs.game_id, _ = uuid.Parse(event.GetId())
			gs.role = roleMap[event.GetMsg()]
			gs.playersAlive = make([]UserInfo, len(event.GetPlayers()))
			gs.playersAlive = GetPlayers(event.GetPlayers())
			if roleMap[event.GetMsg()] == Mafia {
				for i := range gs.playersAlive {
					gs.playersAlive[i].isMafia = No
				}
				for _, y := range ConvertToList(event.GetVotes()) {
					for i, x := range gs.playersAlive {
						if y == x.Nickname {
							gs.playersAlive[i].isMafia = Yes
						}
					}
				}
			} else if roleMap[event.GetMsg()] == Sheriff {
				for i, x := range gs.playersAlive {
					if x.UniqueId == gs.user.UniqueId {
						gs.playersAlive[i].isMafia = No
					}
				}
			}
			client.Start(gs)
			return
		}
	}
}
 
func main() {
	log.Println("Client running ...")
 
	conn, err := grpc.Dial(":50051", grpc.WithInsecure())
	if err != nil {
		log.Fatalln(err)
	}
	defer conn.Close()
 
	cli := mafia.NewMafiaServiceClient(conn)
	client := new(Client)
	client.cli = cli
	client.isBot = false
	client.ctx = context.Background()

	scanner := bufio.NewScanner(os.Stdin)
	fmt.Print("Your nickname: \n\n>>> ")
	scanner.Scan()
	request := &mafia.CreateUser{Username: scanner.Text()}
	fmt.Print("Do you want the bot to play instead of you[y/n]: \n\n>>> ")
	for {
		scanner.Scan()
		ans := scanner.Text()
		if (ans != "y") && (ans != "n") {
			fmt.Print("You should type 'y' or 'n' without brackets\n\n >>")
		} else {
			client.isBot = (ans == "y")
			break
		}
	}
	fmt.Print("Number of games you want to play: \n\n>>> ")
	var numberOfGames int

	_, err = fmt.Scanf("%d", &numberOfGames)
 
	response, err := client.cli.CreateNewUser(client.ctx, request)
	if err != nil {
		log.Fatalln(err)
	}
 
	log.Println("Response:", response.GetId())

	for k := 0; k < numberOfGames; k++ {
		joinMsg := mafia.JoinMsg{UserInfo: response}
		stream, err := client.cli.JoinGame(client.ctx, &joinMsg)
		if err != nil {
			log.Fatalln(err)
		}
		client.stream = stream
		client.playGame(response)
	}
}