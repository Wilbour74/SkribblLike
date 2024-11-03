package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"log"
	"time"
	

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type Client struct {
    ID     string
    Conn   *websocket.Conn
    RoomID string
	Name string
}

type Room struct {
    ID      string
    Clients []*Client
    History []Message
    Creator string
    Word string `json:"count,omitempty"`
    CurrentTurn int
}


var clients = make([]*Client, 0)
var rooms = make(map[string]*Room)
var messageHistory = make([]Message, 0)

type Message struct {
	Type      string  `json:"type"`
	Message   string  `json:"message,omitempty"`
	X         float64 `json:"x,omitempty"`
	Y         float64 `json:"y,omitempty"`
	Color     string  `json:"color,omitempty"`
	LineWidth int     `json:"lineWidth,omitempty"`
	Id        string  `json:"expediteur,omitempty"`
    Count     int     `json:"count,omitempty"`
    Word      string  `json:"word,omitempty"`
}

func generateClientID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func broadcastRoomClientCount(roomID string) {
    if room, ok := rooms[roomID]; ok {
        clientCountMessage := Message{
            Type:    "client_count",
            Message: fmt.Sprintf("%d", len(room.Clients)),
            Count: len(room.Clients),
        }
        jsonMessage, err := json.Marshal(clientCountMessage)
        if err != nil {
            fmt.Println("Erreur lors de la conversion en JSON:", err)
            return
        }
        for _, client := range room.Clients {
            if err := client.Conn.WriteMessage(websocket.TextMessage, jsonMessage); err != nil {
                fmt.Println("Erreur lors de l'envoi du nombre de clients:", err)
            }
        }

        roomCreator := Message{
            Type: "room_creator",
            Message: room.Creator,
        }

        jsonCreator, err := json.Marshal(roomCreator)

        for _, client := range room.Clients{
            if err := client.Conn.WriteMessage(websocket.TextMessage, jsonCreator); err != nil {
                fmt.Println("Erreur lors de l'envoi des détails de la room:", err)
            }
        }
    }
}


func handleWebSocket(w http.ResponseWriter, r *http.Request) {
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        fmt.Println("WebSocket upgrade error:", err)
        return
    }
	name := r.URL.Query().Get("name")
    roomID := r.URL.Query().Get("room")
	log.Printf("Name: %s, RoomID: %s", name, roomID)
    if roomID == "" {
        roomID = generateClientID()
    }

    clientID := generateClientID()
    client := &Client{
        ID:     clientID,
		Name:   name,
        Conn:   conn,
        RoomID: roomID,
    }

    joinRoom(roomID, client)

	welcomeMessage := Message{
        Type:    "connect",
        Message: fmt.Sprintf("%s est dans la place !", client.Name),
    }

    jsonWelcomeMessage, err := json.Marshal(welcomeMessage)
    if err != nil {
        fmt.Println("Erreur lors de la conversion du message en JSON:", err)
        return
    }

    sendToRoom(client.RoomID, websocket.TextMessage, jsonWelcomeMessage)

	broadcastRoomClientCount(roomID)

    defer func() {
        for i, c := range rooms[roomID].Clients {
            if c == client {
                rooms[roomID].Clients = append(rooms[roomID].Clients[:i], rooms[roomID].Clients[i+1:]...)
                break
            }
        }

        disconnectMessage := Message{
            Type:    "disconnect",
            Message: fmt.Sprintf("%s vient de se déconnecter", client.Name),
        }

        jsonDisconnectMessage, err := json.Marshal(disconnectMessage)
        if err != nil {
            fmt.Println("Erreur lors de la conversion du message en JSON:", err)
            return
        }

        sendToRoom(client.RoomID, websocket.TextMessage, jsonDisconnectMessage)
    
		broadcastRoomClientCount(roomID)
        conn.Close()
    }()

    

    for {
        msgType, msg, err := conn.ReadMessage()
        if err != nil {
            fmt.Println("Error reading message:", err)
            return
        }

        var message Message
        if err := json.Unmarshal(msg, &message); err != nil {
            fmt.Println("Erreur lors de la conversion du message JSON:", err)
            continue
        }

        if message.Type == "start_game" {
            room := rooms[client.RoomID]
            room.CurrentTurn = 0
    
            startGameMessage := Message{
                Type:    "game_started",
                Message: fmt.Sprintf("La partie a commencé! C'est au tour de %s.", room.Clients[room.CurrentTurn].Name),
                Word: "Wilfried",
            }
    
            jsonStartGameMessage, err := json.Marshal(startGameMessage)
            if err != nil {
                fmt.Println("Erreur lors de la conversion du message de démarrage du jeu en JSON:", err)
                return
            }
    
            sendToRoom(client.RoomID, websocket.TextMessage, jsonStartGameMessage)
        } else {
            sendToRoom(client.RoomID, msgType, msg)
        }
    }
}



func serveHome(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "index.html")
}

func button(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w,r, "chose.html")
}

func main() {
	http.HandleFunc("/echo", handleWebSocket)
	http.HandleFunc("/", button)
	http.HandleFunc("/skribbl", serveHome)

	fmt.Println("Le serveur tourne sur le port 9090")
	err := http.ListenAndServe(":9090", nil)
	if err != nil {
		fmt.Println("Erreur lors du démarrage du serveur:", err)
	}
}


func joinRoom(roomID string, client *Client) {
    if room, ok := rooms[roomID]; ok {
        room.Clients = append(room.Clients, client)

        for _, msg := range room.History {
            jsonMsg, err := json.Marshal(msg)
            if err != nil {
                fmt.Println("Erreur lors de la conversion en JSON:", err)
                continue
            }
            if err := client.Conn.WriteMessage(websocket.TextMessage, jsonMsg); err != nil {
                fmt.Println("Erreur lors de l'envoi de l'historique:", err)
            }
        }
    } else {
        newRoom := &Room{
            ID:      roomID,
            Clients: []*Client{client},
            History: []Message{},
            Creator: client.Name,
        }
        rooms[roomID] = newRoom
    }
}


func sendToRoom(roomID string, msgType int, msg []byte) {
    if room, ok := rooms[roomID]; ok {
        var message Message
        if err := json.Unmarshal(msg, &message); err != nil {
            fmt.Println("Error unmarshaling message:", err)
            return
        }

        if message.Type != "room_creator" {
            room.History = append(room.History, message)
        } 
        

        for _, c := range room.Clients {
            if err := c.Conn.WriteMessage(msgType, msg); err != nil {
                fmt.Println("Error writing to client:", err)
            }
        }
    }
}
