package main

import (
	"log"
	"net/http"

	"github.com/a-h/templ"
)

func render(w http.ResponseWriter, r *http.Request, c templ.Component) {
	if err := c.Render(r.Context(), w); err != nil {
		log.Printf("render: %v", err)
	}
}
