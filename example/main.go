// +build ignore

package main

import (
	"log"
	"net/http"
)

func main() {
	log.Fatal(http.ListenAndServe("127.0.0.1:8080", http.FileServer(AssetDir("static"))))
}
