package main

import (
	"log"
	"net/http"

	"localslack/internal/utils"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type PlaceHolderConfig struct {
	Addr string
}

func main() {

	cfg := PlaceHolderConfig{
		Addr: utils.GetEnvWithDefault("SLACK_LISTEN_ADDR", ":3232"),
	}

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("welcome"))
	})

	log.Printf("Listening on %s", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, r); err != nil {
		log.Fatal(err)
	}
}
