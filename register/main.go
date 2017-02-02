package main

import (
	"html/template"
	"log"
	"net/http"
)

func main() {
	backends := make(map[string]string)

	http.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		backends[r.FormValue("hostname")] = r.FormValue("address")
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t, err := template.New("registry").Parse(html)
		if err != nil {
			log.Println(err)
		}

		err = t.Execute(w, backends)
		if err != nil {
			log.Println(err)
		}
	})

	log.Fatal(http.ListenAndServe("0.0.0.0:80", nil))
}

var html = `
<!DOCTYPE html>
<html>
  <head>
    <meta charset="UTF-8">
    <title>Registry</title>
  </head>
  <body>
    <h1>Service Registry</h1>
	{{range $hostname, $address := . }}
    <p>{{$hostname}}: {{$address}}</p>
    {{end}}
  <body>
</html>
`
