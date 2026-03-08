package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"stock-options/pkg/api"
)

func main() {
	portFlag := flag.String("port", "8080", "Port to run the server on")
	flag.Parse()

	port := os.Getenv("PORT")
	if port == "" {
		port = *portFlag
	}

	router := api.NewRouter()
	log.Printf("Server starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, router))
}
