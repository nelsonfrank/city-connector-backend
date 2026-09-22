package main

import (
	"fmt"
	"log"
	"net/http"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("200 OK"))
	})
	
	port := "8081"
	fmt.Printf("Routing service listening on port %s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
