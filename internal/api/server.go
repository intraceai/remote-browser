package api

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/intraceai/remote-browser/internal/browser"
)

type Server struct {
	router  *gin.Engine
	browser *browser.Browser
}

type OpenRequest struct {
	URL string `json:"url" binding:"required"`
}

func NewServer(b *browser.Browser) *Server {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())

	s := &Server{
		router:  router,
		browser: b,
	}

	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	s.router.GET("/health", s.healthCheck)
	s.router.POST("/open", s.openURL)
	s.router.POST("/capture", s.capture)
	s.router.POST("/shutdown", s.shutdown)
}

func (s *Server) Run(addr string) error {
	return s.router.Run(addr)
}

func (s *Server) healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *Server) openURL(c *gin.Context) {
	var req OpenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := s.browser.OpenURL(req.URL); err != nil {
		log.Printf("Failed to open URL: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *Server) capture(c *gin.Context) {
	result, err := s.browser.Capture()
	if err != nil {
		log.Printf("Failed to capture: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

func (s *Server) shutdown(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "shutting down"})

	go func() {
		s.browser.Close()
	}()
}
