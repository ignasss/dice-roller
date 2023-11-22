package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"

	"github.com/gorilla/websocket"
	uuid "github.com/satori/go.uuid"
)

type ClientManager struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
}

type Client struct {
	id     string
	socket *websocket.Conn
	send   chan []byte
}

type Message struct {
	Sender    string `json:"sender,omitempty"`
	Recipient string `json:"recipient,omitempty"`
	Content   string `json:content,omitempty`
	ServerIP  string `json:serverIp,omitempty`
	SenderIP  string `json:senderIp,omitempty`
}

var manager = ClientManager{
	broadcast:  make(chan []byte),
	register:   make(chan *Client),
	unregister: make(chan *Client),
	clients:    make(map[*Client]bool),
}

func (manager *ClientManager) start() {
	for {
		select {
		case conn := <-manager.register:
			manager.clients[conn] = true
			jm, _ := json.Marshal(&Message{Content: "A new socket has connected.", ServerIP: LocalIp(), SenderIP: conn.socket.RemoteAddr().String()})
			manager.send(jm, conn)
		case conn := <-manager.unregister:
			if _, ok := manager.clients[conn]; ok {
				close(conn.send)
				delete(manager.clients, conn)
				jm, _ := json.Marshal(&Message{Content: "A socket has disconnected.", ServerIP: LocalIp(), SenderIP: conn.socket.RemoteAddr().String()})
				manager.send(jm, conn)
			}
		case message := <-manager.broadcast:
			for conn := range manager.clients {
				select {
				case conn.send <- message:
				default:
					close(conn.send)
					delete(manager.clients, conn)
				}
			}
		}
	}
}

func (manager *ClientManager) send(message []byte, ignore *Client) {
	for client := range manager.clients {
		if client != ignore {
			client.send <- message
		}
	}
}

func (c *Client) read() {
	defer func() {
		manager.unregister <- c
		_ = c.socket.Close()
	}()

	for {
		_, message, err := c.socket.ReadMessage()
		if err != nil {
			fmt.Printf("read error, err: %v \n", err)
			manager.unregister <- c
			_ = c.socket.Close()
			break
		}

		jm, _ := json.Marshal(&Message{Sender: c.id, Content: string(message), ServerIP: LocalIp(), SenderIP: c.socket.RemoteAddr().String()})
		manager.broadcast <- jm
	}
}

func (c *Client) write() {
	defer func() {
		_ = c.socket.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				var err = c.socket.WriteMessage(websocket.CloseMessage, []byte{})
				fmt.Printf("write error: %v \n", err)
				return
			}

			var err = c.socket.WriteMessage(websocket.TextMessage, message)
			if err != nil {
				fmt.Printf("send err: %v \n", err)
			}
		}
	}
}

func main() {
	fmt.Println("Starting application...")
	go manager.start()
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		filename := "./templates/index.html"
		body, err := os.ReadFile(filename)
		if err != nil {
			w.Write([]byte(err.Error()))
		}

		w.Write(body)
	})
	http.HandleFunc("/ws", wsHandler)
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/favicon.ico", doNothing)
	fmt.Println("dice roller start.....")
	_ = http.ListenAndServe("0.0.0.0:5069", nil)
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024 * 1024 * 1024,
	WriteBufferSize: 1024 * 1024 * 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

func wsHandler(res http.ResponseWriter, req *http.Request) {
	conn, err := upgrader.Upgrade(res, req, nil)
	if err != nil {
		fmt.Printf("error during upgrade: %v", err)
		http.NotFound(res, req)
		return
	}

	client := &Client{id: uuid.Must(uuid.NewV4(), nil).String(), socket: conn, send: make(chan []byte)}
	manager.register <- client
	go client.write()
	go client.read()
}

func healthHandler(res http.ResponseWriter, _ *http.Request) {
	_, _ = res.Write([]byte("ok"))
}

func LocalIp() string {
	address, _ := net.InterfaceAddrs()
	var ip = "localhost"
	for _, address := range address {
		if ipAddress, ok := address.(*net.IPNet); ok && !ipAddress.IP.IsLoopback() {
			if ipAddress.IP.To4() != nil {
				ip = ipAddress.IP.String()
			}
		}
	}

	return ip
}

func doNothing(res http.ResponseWriter, req *http.Request) {
}
