package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/net/websocket"
)

type hub struct {
	clients          map[string]*websocket.Conn
	addClientChan    chan *websocket.Conn
	removeClientChan chan *websocket.Conn
	broadcastChan    chan string
	rand             *rand.Rand
	dice             []int
}

var (
	port = flag.String("port", "5069", "port used for ws connection")
)

func main() {
	flag.Parse()
	log.Fatal(server(*port))
}

func server(port string) error {
	h := newHub()
	router := gin.Default()
	router.LoadHTMLGlob("templates/*.html")
	router.GET("/", homePage)
	router.GET("/favicon.ico", doNothing)
	router.GET("/dice", func(c *gin.Context) {
		handler := websocket.Handler(func(ws *websocket.Conn) {
			webSocketHandler(ws, h)
		})
		handler.ServeHTTP(c.Writer, c.Request)
	})
	return router.Run("localhost:" + port)
}

func newHub() *hub {
	return &hub{
		clients:          make(map[string]*websocket.Conn),
		addClientChan:    make(chan *websocket.Conn),
		removeClientChan: make(chan *websocket.Conn),
		broadcastChan:    make(chan string),
		rand:             rand.New(rand.NewSource(time.Now().UnixNano())),
		dice:             []int{1, 2, 3, 4, 5, 6},
	}
}

func homePage(c *gin.Context) {
	c.HTML(http.StatusOK, "index.html", nil)
}

func doNothing(c *gin.Context) {
}

func webSocketHandler(ws *websocket.Conn, h *hub) {
	go h.run()
	h.addClientChan <- ws
	for {
		var m string
		err := websocket.Message.Receive(ws, &m)
		if err != nil {
			h.broadcastChan <- err.Error()
			h.removeClient(ws)
			return
		}

		h.broadcastChan <- m
	}
}

func (h *hub) run() {
	for {
		select {
		case conn := <-h.addClientChan:
			h.addClient(conn)
		case conn := <-h.removeClientChan:
			h.removeClient(conn)
		case m := <-h.broadcastChan:
			h.broadcastMessage(m)
		}
	}
}

func (h *hub) addClient(conn *websocket.Conn) {
	h.clients[conn.RemoteAddr().String()] = conn
}

func (h *hub) removeClient(conn *websocket.Conn) {
	delete(h.clients, conn.LocalAddr().String())
}

func (h *hub) broadcastMessage(m string) {
	d1 := h.rollDice()
	d2 := h.rollDice()
	res := d1 + d2
	fr := fmt.Sprintf("%v %v + %v = %v", m, d1, d2, res)
	for _, conn := range h.clients {
		err := websocket.Message.Send(conn, fr)
		if err != nil {
			fmt.Println("Error broadcasting message: ", err)
			return
		}
	}
}

func (h *hub) rollDice() int {
	return h.dice[h.rand.Intn(len(h.dice))]
}
