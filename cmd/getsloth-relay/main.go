// Command getsloth-relay runs the getsloth WebSocket relay server.
// This is the piece that eventually deploys to the founder-provided
// server per tasks/plan.md's Task L3 - this file is the minimal runnable
// entrypoint B4 needs to actually exercise the host<->relay<->viewer
// flow end to end, not the deployment setup itself.
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/arinprajapati/getsloth/internal/relay"
)

func main() {
	addr := os.Getenv("GETSLOTH_RELAY_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	srv := relay.NewServer()
	log.Printf("getsloth relay listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, srv.Handler()))
}
