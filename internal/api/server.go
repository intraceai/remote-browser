package api

import (
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/intraceai/remote-browser/internal/chrome"
)

const minFrameInterval = 40 * time.Millisecond // Max ~25 fps

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for now
	},
}

type Server struct {
	router    *gin.Engine
	chrome    *chrome.Chrome
	clients   map[*websocket.Conn]bool
	mu        sync.RWMutex
	lastFrame time.Time
	frameMu   sync.Mutex
}

type OpenRequest struct {
	URL string `json:"url" binding:"required"`
}

type InputEvent struct {
	Type   string  `json:"type"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Button int     `json:"button"`
	Key    string  `json:"key"`
	Text   string  `json:"text"`
	DeltaX float64 `json:"deltaX"`
	DeltaY float64 `json:"deltaY"`
}

type FrameMessage struct {
	Type string `json:"type"`
	Data string `json:"data"` // base64 encoded JPEG
}

func NewServer() (*Server, error) {
	s := &Server{
		clients: make(map[*websocket.Conn]bool),
	}

	// Create Chrome instance with frame callback
	c, err := chrome.New(s.onFrame)
	if err != nil {
		return nil, err
	}
	s.chrome = c

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(corsMiddleware())

	s.router = router
	s.setupRoutes()

	return s, nil
}

func (s *Server) onFrame(data []byte) {
	// Throttle frames
	s.frameMu.Lock()
	now := time.Now()
	if now.Sub(s.lastFrame) < minFrameInterval {
		s.frameMu.Unlock()
		return
	}
	s.lastFrame = now
	s.frameMu.Unlock()

	s.mu.RLock()
	clients := make([]*websocket.Conn, 0, len(s.clients))
	for c := range s.clients {
		clients = append(clients, c)
	}
	s.mu.RUnlock()

	if len(clients) == 0 {
		return
	}

	msg := FrameMessage{
		Type: "frame",
		Data: base64.StdEncoding.EncodeToString(data),
	}
	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return
	}

	for _, c := range clients {
		if err := c.WriteMessage(websocket.TextMessage, msgBytes); err != nil {
			s.mu.Lock()
			delete(s.clients, c)
			s.mu.Unlock()
			c.Close()
		}
	}
}

func (s *Server) setupRoutes() {
	s.router.GET("/health", s.healthCheck)
	s.router.GET("/ws", s.handleWebSocket)
	s.router.POST("/open", s.openURL)
	s.router.POST("/capture", s.capture)
	s.router.POST("/start-stream", s.startStream)
	s.router.POST("/stop-stream", s.stopStream)
}

func (s *Server) Run(addr string) error {
	return s.router.Run(addr)
}

func (s *Server) Close() error {
	s.mu.Lock()
	for c := range s.clients {
		c.Close()
	}
	s.mu.Unlock()

	if s.chrome != nil {
		return s.chrome.Close()
	}
	return nil
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

func (s *Server) healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *Server) handleWebSocket(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	s.mu.Lock()
	s.clients[conn] = true
	s.mu.Unlock()

	log.Printf("WebSocket client connected")

	defer func() {
		s.mu.Lock()
		delete(s.clients, conn)
		s.mu.Unlock()
		conn.Close()
		log.Printf("WebSocket client disconnected")
	}()

	// Handle incoming messages (input events)
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var event InputEvent
		if err := json.Unmarshal(msg, &event); err != nil {
			continue
		}

		s.handleInput(event)
	}
}

func (s *Server) handleInput(event InputEvent) {
	switch event.Type {
	case "mousemove":
		s.chrome.MouseMove(event.X, event.Y)
	case "mousedown", "click":
		button := "left"
		if event.Button == 2 {
			button = "right"
		} else if event.Button == 1 {
			button = "middle"
		}
		s.chrome.MouseClick(event.X, event.Y, button)
	case "keydown":
		s.chrome.KeyPress(event.Key)
	case "text":
		s.chrome.TypeText(event.Text)
	case "wheel":
		s.chrome.Scroll(event.X, event.Y, event.DeltaX, event.DeltaY)
	}
}

func (s *Server) openURL(c *gin.Context) {
	var req OpenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := s.chrome.Navigate(req.URL); err != nil {
		log.Printf("Failed to open URL: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *Server) capture(c *gin.Context) {
	result, err := s.chrome.Capture()
	if err != nil {
		log.Printf("Failed to capture: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"screenshot": base64.StdEncoding.EncodeToString(result.Screenshot),
		"dom":        result.DOM,
		"final_url":  result.FinalURL,
		"viewport": gin.H{
			"width":  result.Width,
			"height": result.Height,
		},
	})
}

func (s *Server) startStream(c *gin.Context) {
	if err := s.chrome.StartScreencast(c.Request.Context()); err != nil {
		log.Printf("Failed to start screencast: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "streaming"})
}

func (s *Server) stopStream(c *gin.Context) {
	if err := s.chrome.StopScreencast(c.Request.Context()); err != nil {
		log.Printf("Failed to stop screencast: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "stopped"})
}
