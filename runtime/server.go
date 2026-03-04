package runtime

import (
	"fmt"
	"net/http"
)

type Server struct {
	Router *Router
	Port   int
}

func NewServer() *Server {
	return &Server{
		Router: NewRouter(),
		Port:   8080,
	}
}

func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.Port)
	fmt.Printf("\033[1;36m✦ httpdsl server listening on port %d\033[0m\n", s.Port)
	return http.ListenAndServe(addr, s.Router)
}
