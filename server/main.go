package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
)

var html = `
<!DOCTYPE html>
<html>
  <head>
    <meta charset="UTF-8">
    <title>Dynamic Port Server</title>
  </head>
  <body>
    <h1>%s</h1>
    <p>%s</p>
  <body>
</html>
`

func main() {
	podName := os.Getenv("POD_NAME")
	podIP := os.Getenv("POD_IP")
	register := os.Getenv("REGISTER_ADDR")

	ln, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		log.Fatal(err)
	}

	hostPort := ln.Addr().String()
	_, port, err := net.SplitHostPort(hostPort)
	if err != nil {
		log.Fatal(err)
	}

	advertisedAddr := net.JoinHostPort(podIP, port)

	log.Println("Registering advertised endpoint...")
	log.Println(advertisedAddr)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, html, podName, advertisedAddr)
	})

	_, err = http.Get(fmt.Sprintf("http://%s/register?hostname=%s&address=%s", register, podName, advertisedAddr))
	if err != nil {
		log.Fatal(err)
	}

	s := &http.Server{}
	go log.Fatal(s.Serve(ln))

	ln.Close()
}
