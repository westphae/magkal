package main

import (
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"sync"
)


type templateHandler struct {
	once     sync.Once
	filename string
	template *template.Template // template represents a single template
}

// ServeHTTP handles the HTTP request.
func (t *templateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	t.once.Do(func() {
		t.template = template.Must(template.ParseFiles(filepath.Join("res", t.filename)))
	})
	if err := t.template.Execute(w, r); err != nil {
		log.Fatalf("error executing template: %s\n", err.Error())
	}
}

func main() {
	http.Handle("/", &templateHandler{filename: "index.html"})
	log.Fatal(http.ListenAndServe(":8000", nil))
}
