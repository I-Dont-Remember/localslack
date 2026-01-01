package api

import (
	"encoding/json"
	"log"
	"net/http"

	"localslack/internal/utils"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type SlackServer struct {
	router *chi.Mux
	deps   utils.Deps
}

func NewServer() *SlackServer {
	s := &SlackServer{
		router: chi.NewRouter(),
		deps:   nil,
	}

	// --- global middleware ---
	s.router.Use(middleware.Logger)          // structured logging
	s.router.Use(middleware.Recoverer)       // panic protection
	s.router.Use(middleware.RedirectSlashes) // /todos/ -> /todos
	s.router.Use(AuthenticateToken)
	s.routes()
	return s
}

func (s *SlackServer) routes() {
	r := s.router

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<h1>LocalSlack</h1>"))
	})

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	r.Route("/api", func(r chi.Router) {
		for methodName, spec := range SlackMethodRegistry {
			path := "/" + methodName
			log.Printf("Registered: %s\n", "/api"+path)
			r.MethodFunc(spec.Verb, path, s.wrap(methodName, spec.Handler))
		}
	})
}

func (s *SlackServer) Start(port string) {
	log.Printf("Listening on %s", port)
	if err := http.ListenAndServe(port, s.router); err != nil {
		log.Fatal(err)
	}
}
