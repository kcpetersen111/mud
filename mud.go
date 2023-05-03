package main

import "fmt"
import "bufio"
import "strings"
import "database/sql"
import "log"
import _ "github.com/mattn/go-sqlite3"
import "net"

type Zone struct {
	ID    int
	Name  string
	Rooms []*Room
}

type Room struct {
	ID          int
	Zone        *Zone
	Name        string
	Description string
	Exits       [6]*Exit
}

type Exit struct {
	FromRoom    *Room
	ToRoom		*Room
	Description string
	Direction	string
}
type Player struct {
	Location	*Room
	Conn		net.Conn
	Name		string
	PlayerScanner *bufio.Scanner 
}
type playertomud struct{
	Command string
	p1 *Player
}
type MudToPlayer struct {
	Response string 
	p1 *Player
}


func main() {

	// the path to the database--this could be an absolute path
		path := "world.db"
		options := "?" + "_busy_timeout=10000" + "&" + "_foreign_keys=ON"
		db, err := sql.Open("sqlite3", path+options)
		defer db.Close()
		if err != nil {
			// handle the error here
			fmt.Println("An error opening the database has occured. Program Shuting down")
			return
		}


	//loads the zones
		tx,err := db.Begin()
		zones := make(map[int]*Zone)
		err = BuildZones(&zones, tx)
		if err != nil {
			tx.Rollback()
			log.Fatalf("%v", err)
		}else {
			tx.Commit()
		}

	//loads the rooms
		tx, err = db.Begin()
		rooms, err:= BuildRooms(&zones, tx)	
		
		if err != nil{
			tx.Rollback()	
			log.Fatalf("%v", err)
		} else{
			tx.Commit()
		}


	//loads the exits
		tx, err = db.Begin()

		err = BuildExits(&rooms, tx)
		if err != nil{
			tx.Rollback()	
			log.Fatalf("%v", err)
		} else{
			tx.Commit()
		}
	
	
	
	ptm := make(chan playertomud)
	mudtoplayer := make(chan MudToPlayer)
	defer close(ptm)
	defer close(mudtoplayer)
	
	players := make([]Player,0)
	go acceptConn(&players, &rooms, ptm,mudtoplayer)

	for {	

		command := <- ptm
		if command.p1 != nil {
			text := strings.Fields(command.Command)
			var response string
			if len(text) == 0 {
				response = fmt.Sprintln("No text provided")

			} else {

				temp := strings.ToLower(text[0])
				switch temp {
				case "l", "lo", "loo", "look":
					response = DoLook(command.p1,text)
				case "n", "no", "nor", "nort", "north":
					response = DoMoveNorth(command.p1)
				case "s", "so", "sou", "sout", "south":
					response = DoMoveSouth(command.p1)
				case "e", "ea", "eas", "east":
					response = DoMoveEast(command.p1)
				case "w", "we", "wes", "west":
					response = DoMoveWest(command.p1)
				case "u","up":
					response = DoMoveUp(command.p1)
				case "d","do","dow","down":
					response = DoMoveDown(command.p1)
				case "r","re","rec","reca","recal","recall":
					response = DoRecall(command.p1, &rooms)
				case "quit":	
					DoQuit(command.p1)
				// 	return	//this would only work in a single player enviorment single user dungeon sud
				case "gossip":
					DoGossip(command.p1, text, players)
				case "tell":
					DoTell(command.p1, text, players)
				case "zone":
					DoZone(command.p1, players)
				case "room":
					DoRoom(command.p1, players)
				case "emote":
					DoEmote(command.p1,players,text)


				default:
					response = fmt.Sprintf( "%s is an unrecognised command, Try again.\n", text[0])
				}
			}
			response+="\n"
			mtp:= MudToPlayer{
				Response: response,
				p1: command.p1,
			}
			mudtoplayer <- mtp

		}
	}	
}


func BuildZones(zones *map[int]*Zone,  tx *sql.Tx) error {
	var id int
	var name string
	
	rows, err := tx.Query("SELECT id, name FROM zones ORDER BY id")
	defer rows.Close()
	if err != nil || rows ==nil{
		return fmt.Errorf("error loading the database %v", err)
	}
	for rows.Next(){
		
		err = rows.Scan(&id,&name)
		if err != nil{
			return fmt.Errorf("error loading the database %v", err)
		}
		z := Zone{ID: id,
				Name: name}
			
		(*zones)[z.ID] = &z
		//fmt.Printf("%v %v",z.ID,z.Name)
	}
	return nil
}

func BuildRooms(zones *map[int]*Zone, tx *sql.Tx) (map[int]*Room, error){

	rm:= make(map[int]*Room)
	var id,zone_id int
	var name, description string
	rows,err :=tx.Query("select id, zone_id, name, description from rooms ;")
	defer rows.Close()
	if err != nil || rows ==nil{
		return nil, fmt.Errorf("error loading the database %v", err)
	}
	for rows.Next(){
		
		err = rows.Scan(&id,&zone_id,&name,&description)
		if err != nil{
			return nil, fmt.Errorf("error loading the database %v", err)
		}
		//z := Zone{ID: id,
			//Name: name}
		// jesusJustWork := Exit{}
		 exi:= [6]*Exit{nil,nil,nil,nil,nil,nil}
		r := Room{ID: id,
				Zone: (*zones)[zone_id],
				Name:name,
				Description:description,
				Exits: exi}
	//	fmt.Println(rows.Next())
		rm[r.ID] = &r
		(*zones)[zone_id].Rooms = append((*zones)[zone_id].Rooms, &r)
		//fmt.Printf("%v %v",z.ID,z.Name)
	}
	return rm, nil

}

func BuildExits(rooms *map[int]*Room, tx *sql.Tx) ( error){
	dirmap := make(map[string]int)
	dirmap["n"] = 0
	dirmap["e"] = 1
	dirmap["w"] = 2
	dirmap["s"] = 3
	dirmap["u"] = 4
	dirmap["d"] = 5
	// 	0 → north
	// 1 → east
	// 2 → west
	// 3 → south
	// 4 → up
	// 5 → down

	var from_room_id, to_room_id int
	var direction, description string
	rows,err := tx.Query("select from_room_id, to_room_id, direction, description from exits;")
	defer rows.Close()
	if err != nil {
		return fmt.Errorf("error loading the database %v", err)
	}

	for rows.Next(){
		err = rows.Scan(&from_room_id,&to_room_id,&direction,&description)
		if err != nil{
			return  fmt.Errorf("error loading the database %v", err)
		}

		//z := Zone{ID: id,
			//Name: name}
		toro := (*rooms)[to_room_id]
		frro := (*rooms)[from_room_id]
		e := Exit{
			FromRoom: frro,
			ToRoom: toro,
			Description: description,
			Direction: direction}
		temp:= dirmap[direction]
		frro.Exits[temp] = &e
	}

	return nil

}


func acceptConn(players *[]Player, rooms *map[int]*Room, ptm chan playertomud, mudtoplayer chan MudToPlayer){
	//accepts play connections and then dispatches the go routine
	ln, err := net.Listen("tcp",":9001")
	if err != nil {
		fmt.Printf("Failed to start up the server, %v",err)
		return
	}
	for {
		conn, err := ln.Accept()
		if err != nil{
			fmt.Println("connection failed")
		}

		
		
		p1 := Player{Location: (*rooms)[3001],
			Conn: conn}

		fmt.Fprintf(conn, "What is your name? \n")
		// fmt.Fprintf(conn, "end of input\n")		//only needed of the client is the custom built client
		
		scanner := bufio.NewScanner(p1.Conn)	 
		p1.PlayerScanner = scanner
		p1.PlayerScanner.Scan()
		p1.Name = p1.PlayerScanner.Text()
		log.Printf("%s has joined the game\n",p1.Name)
		// fmt.Printf("%s has joined the game\n",p1.Name)
		*players = append(*players, p1)
		// playerChan <- p1
		go handlePlayerInput(p1,p1.Conn,ptm,mudtoplayer)
		// go handlePlayerOutput
		//players = append(players, p1)
		//go newPlayer(p1,conn, &rooms)
	}
}
func handlePlayerInput(player1 Player,conn net.Conn, ptm chan playertomud, mudtoplayer chan MudToPlayer){
	
	
		
	// scanner := bufio.NewScanner(player1.Conn)	 
	// scanner.Scan()
	// fmt.Println("made it this far")

	for {
		fmt.Fprintln(conn, "What do you want to do next? ")
		// fmt.Fprintf(conn, "end of input\n")			//only needed of the client is the custom built client
	 
		player1.PlayerScanner.Scan()	
		com := player1.PlayerScanner.Text()
		newPTM := playertomud{
			Command: com,
			p1: &player1,
		}
		ptm <- newPTM
		
		playerdata := <-mudtoplayer
		fmt.Fprintln(conn, playerdata.Response)
		// fmt.Fprintf(conn, "end of input\n")			//only needed of the client is the custom built client
	}
}

func DoLook(p1 *Player, text []string) (string) {
	if len(text) == 1 {
		resp := ""
		resp += "Location: " + p1.Location.Name + "\n"
		resp += "Description: " + p1.Location.Description + " "
		for _,val := range p1.Location.Exits{
			if val !=nil{
				resp += fmt.Sprintf("%s ", val.Direction)
			}
		}
		return fmt.Sprintln(resp)
		// fmt.Fprintln(p1.Conn, resp)
	} else if len(text) >1{
		switch text[1][:1]{
			case "n":
				if p1.Location.Exits[0] != nil{
					resp := ""
					resp += fmt.Sprintln("You look north and see: ", p1.Location.Exits[0].ToRoom.Name)
					resp += fmt.Sprintln("Description: ", p1.Location.Exits[0].ToRoom.Description)
					
					for _,val := range p1.Location.Exits[0].ToRoom.Exits{
						if val !=nil{
							resp += fmt.Sprintf("%s ", val.Direction)
						}
					}
					// fmt.Fprintln(p1.Conn,resp)
					return fmt.Sprintln(resp)

				} else{
					return fmt.Sprintln("There is no exit in that direction")

					// fmt.Fprintln(p1.Conn,)
				}
			case "e":
				if p1.Location.Exits[1] != nil{
					resp := ""
					resp += fmt.Sprintln("You look east and see: ", p1.Location.Exits[1].ToRoom.Name)
					resp += fmt.Sprintln("Description: ", p1.Location.Exits[1].ToRoom.Description)
					
					for _,val := range p1.Location.Exits[1].ToRoom.Exits{
						if val !=nil{
							resp += fmt.Sprintf("%s ", val.Direction)
						}
					}
					// fmt.Fprintln(p1.Conn,resp)
					return fmt.Sprintln(resp)


				} else{
					return fmt.Sprintln("There is no exit in that direction")
					// fmt.Fprintln(p1.Conn,)
				}
			case "w":
				if p1.Location.Exits[2] != nil{
					resp := ""
					resp += fmt.Sprintln("You look west and see: ", p1.Location.Exits[2].ToRoom.Name)
					resp += fmt.Sprintln("Description: ", p1.Location.Exits[2].ToRoom.Description)
					
					for _,val := range p1.Location.Exits[2].ToRoom.Exits{
						if val !=nil{
							resp += fmt.Sprintf("%s ", val.Direction)
						}
					}
					// fmt.Fprintln(p1.Conn,resp)
					return fmt.Sprintln(resp)


				} else{
					return fmt.Sprintln("There is no exit in that direction")

					// fmt.Fprintln(p1.Conn, )
				}
			case "s":
				if p1.Location.Exits[3] != nil{
					resp := ""
					resp += fmt.Sprintln("You look south and see: ", p1.Location.Exits[3].ToRoom.Name)
					resp += fmt.Sprintln("Description: ", p1.Location.Exits[3].ToRoom.Description)
					
					for _,val := range p1.Location.Exits[3].ToRoom.Exits{
						if val !=nil{
							resp += fmt.Sprintf("%s ", val.Direction)
						}
					}
					return fmt.Sprintln(resp)

				} else{
					return fmt.Sprintln("There is no exit in that direction")
				}
			case "u":
				if p1.Location.Exits[4] != nil{
					resp := ""
					resp += fmt.Sprintln("You look up and see: ", p1.Location.Exits[4].ToRoom.Name)
					resp += fmt.Sprintln("Description: ", p1.Location.Exits[4].ToRoom.Description)
					
					for _,val := range p1.Location.Exits[4].ToRoom.Exits{
						if val !=nil{
							resp += fmt.Sprintf("%s ", val.Direction)
						}
					}
					return fmt.Sprintln(resp)
					// fmt.Fprintln(p1.Conn,resp)

				} else{
					// fmt.Fprintln(p1.Conn,"There is no exit in that direction")
					return fmt.Sprintln("There is no exit in that direction")

				}
			case "d":
				if p1.Location.Exits[5] != nil{
					resp := ""
					resp += fmt.Sprintln("You look down and see: ", p1.Location.Exits[5].ToRoom.Name)
					resp +=fmt.Sprintln("Description: ", p1.Location.Exits[5].ToRoom.Description)
					
					for _,val := range p1.Location.Exits[5].ToRoom.Exits{
						if val !=nil{
							resp += fmt.Sprintf("%s ", val.Direction)
						}
					}
					return fmt.Sprintln(resp)

					// fmt.Fprintln(p1.Conn,resp)

				} else{
					fmt.Fprintln(p1.Conn,"There is no exit in that direction")
					return fmt.Sprintln("There is no exit in that direction")

				}

		}
	}
	return "unexpected error"
}

func DoTell(p1 *Player, text []string, playerList []Player){
	if len(text)<2{
		return 
	}
	for _,p := range playerList{
		if p.Name == text[1]{
			fmt.Fprintf(p.Conn, "%s tells you: %s\n" ,p1.Name, strings.Join(text[2:], " "))
			return
		}
	}

	fmt.Fprintf(p1.Conn, "%s is not a recognised name\n" ,text[1])
}
func DoGossip(p1 *Player, text []string, playerList []Player){
	for _,p := range playerList{
		fmt.Fprintf(p.Conn, "%s gossips: %s\n" ,p1.Name, strings.Join(text[1:], " "))
		// fmt.Fprintf(p.Conn, "end of input\n")	//only needed of the client is the custom built client
	}
}
func DoZone(p1 *Player, playerList []Player){
	//finds all of the players in the same zone
	for _, p := range playerList{
		if p.Location.Zone.ID == p1.Location.Zone.ID && p.Name != p1.Name{
			fmt.Fprintf(p1.Conn, "%s is in your zone\n" ,p.Name)
		}
	}
}
func DoEmote(p1 *Player, playerList []Player, text []string){
	if len(text)!=3{
		return 
	}
	for _,p := range playerList{
		if p.Name == text[1]{
			fmt.Fprintf(p.Conn, "%s %s at you\n" ,p1.Name, text[2])
			return
		}
	}

	fmt.Fprintf(p1.Conn, "%s is not a recognised name\n" ,text[1])
}
func DoRoom(p1 *Player, playerList []Player){
	//finds all of the players in the same room
	for _, p := range playerList{
		if p.Location.ID == p1.Location.ID && p.Name != p1.Name{
			fmt.Fprintf(p1.Conn, "%s is in your room\n" ,p.Name)
		}
	}
}
func DoMoveNorth(p1 *Player) (string) {
	if p1.Location.Exits[0] != nil{
		p1.Location = p1.Location.Exits[0].ToRoom
		return fmt.Sprintln(p1.Location.Description)
		// fmt.Fprintln(p1.Conn,p1.Location.Description)
	} else {
		return fmt.Sprintln("There is no exit in that direction")
		// fmt.Fprintln(p1.Conn,"There is no exit in that direction")
	}
}
func DoMoveSouth(p1 *Player) (string) {
	if p1.Location.Exits[3] != nil{
		p1.Location = p1.Location.Exits[3].ToRoom
		return fmt.Sprintln(p1.Location.Description)

		// fmt.Fprintln(p1.Conn,p1.Location.Description)
	} else {
		return fmt.Sprintln("There is no exit in that direction")

		// fmt.Fprintln(p1.Conn,"There is no exit in that direction")
	}
}
func DoMoveEast(p1 *Player) (string) {

	if p1.Location.Exits[1] != nil{
		p1.Location = p1.Location.Exits[1].ToRoom
		return fmt.Sprintln(p1.Location.Description)

		// fmt.Fprintln(p1.Conn,p1.Location.Description)
	} else {
		return fmt.Sprintln("There is no exit in that direction")

		// fmt.Fprintln(p1.Conn,"There is no exit in that direction")
	}
}
func DoMoveWest(p1 *Player) (string) {
	if p1.Location.Exits[2] != nil{
		p1.Location = p1.Location.Exits[2].ToRoom
		return fmt.Sprintln(p1.Location.Description)

		// fmt.Fprintln(p1.Conn,p1.Location.Description)
	} else {
		return fmt.Sprintln("There is no exit in that direction")

		// fmt.Fprintln(p1.Conn,"There is no exit in that direction")
	}
}
func DoMoveUp(p1 *Player) string {
	if p1.Location.Exits[4] != nil{
		p1.Location = p1.Location.Exits[4].ToRoom
		return fmt.Sprintln(p1.Location.Description)

		// fmt.Fprintln(p1.Conn,p1.Location.Description)
	} else {
		return fmt.Sprintln("There is no exit in that direction")

		// fmt.Fprintln(p1.Conn,"There is no exit in that direction")
	}
}
func DoMoveDown(p1 *Player) string {
	if p1.Location.Exits[5] != nil{
		p1.Location = p1.Location.Exits[5].ToRoom
		return fmt.Sprintln(p1.Location.Description)

		// fmt.Fprintln(p1.Conn,p1.Location.Description)
	} else {
		return fmt.Sprintln("There is no exit in that direction")

		// fmt.Fprintln(p1.Conn,"There is no exit in that direction")
	}
}
func DoRecall(p1 *Player,m *map[int]*Room) (string){

	p1.Location = (*m)[3001]
	return fmt.Sprintln("You think about your time in the temple and find yourself back inside of it")
	// fmt.Fprintln(p1.Conn, "You think about your time in the temple and find yourself back inside of it")
}
func DoQuit(p1 *Player){
	log.Printf("%s has left the game",p1.Name)
	fmt.Fprintf(p1.Conn, "end of communication\n")
	p1.Conn.Close()
	p1 = nil
	// p1.PlayerScanner.Close()
	
}
