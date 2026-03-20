package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"mdhub/go-api/internal/app"
	"mdhub/go-api/internal/db"
)

func main() {
	database, err := db.Open("../../data/db/app.db")
	if err != nil {
		log.Fatalf("open db failed: %v", err)
	}
	defer database.Close()

	if err := db.InitSchema(database); err != nil {
		log.Fatalf("init schema failed: %v", err)
	}

	apiToken := os.Getenv("MDHUB_API_TOKEN")
	port := os.Getenv("MDHUB_API_PORT")
	if port == "" {
		port = "8080"
	}

	srv := app.NewServer(database, "../../data/files", apiToken)
	addr := fmt.Sprintf(":%s", port)
	log.Printf("api listening on %s", addr)
	if err := http.ListenAndServe(addr, srv.Routes()); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}
