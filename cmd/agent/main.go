package main

import (
	"log"
	"net/http"
	"perfolizer/pkg/agent"
	"perfolizer/pkg/config"
)

func main() {
	cfgPath := config.ResolveAgentConfigPath()
	cfg, err := config.LoadAgentConfig(cfgPath)
	if err != nil {
		log.Fatalf("failed to load agent config %q: %v", cfgPath, err)
	}

	srv := agent.NewServer()
	addr := cfg.ListenAddr()

	log.Printf("Perfolizer agent listening on %s (config: %s)", addr, cfgPath)
	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		log.Fatalf("agent server failed: %v", err)
	}
}
