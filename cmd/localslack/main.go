package main

import (
	"log"

	"localslack/internal/utils"
	"localslack/internal/api"
)

type PlaceHolderConfig struct {
	Addr string
}

func main() {
	log.Println("LocalSlack booting up...")

	cfg := PlaceHolderConfig{
		Addr: utils.GetEnvWithDefault("SLACK_LISTEN_ADDR", ":3232"),
	}

	s := api.NewServer()

	s.Start(cfg.Addr)
}
