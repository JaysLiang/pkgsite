// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/discovery/internal/frontend"
	"golang.org/x/discovery/internal/middleware"
	"golang.org/x/discovery/internal/postgres"
)

const handlerTimeout = 1 * time.Minute

var (
	user     = getEnv("GO_DISCOVERY_DATABASE_USER", "postgres")
	password = getEnv("GO_DISCOVERY_DATABASE_PASSWORD", "")
	host     = getEnv("GO_DISCOVERY_DATABASE_HOST", "localhost")
	dbname   = getEnv("GO_DISCOVERY_DATABASE_NAME", "discovery-database")
	dbinfo   = fmt.Sprintf("user=%s password=%s host=%s dbname=%s sslmode=disable", user, password, host, dbname)

	staticPath = flag.String("static", "content/static", "path to folder containing static files served")
)

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func main() {
	flag.Parse()

	db, err := postgres.Open(dbinfo)
	if err != nil {
		log.Fatalf("postgres.Open(user=%s host=%s db=%s): %v", user, host, dbname, err)
	}
	defer db.Close()

	templateDir := filepath.Join(*staticPath, "html")
	controller, err := frontend.New(db, templateDir)
	if err != nil {
		log.Fatalf("frontend.New(db, %q): %v", templateDir, err)
	}

	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(*staticPath))))
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, fmt.Sprintf("%s/img/favicon.ico", http.Dir(*staticPath)))
	})
	mux.HandleFunc("/search/", controller.HandleSearch)
	mux.HandleFunc("/", controller.HandleDetails)

	mw := middleware.Timeout(handlerTimeout)

	// Default to addr on localhost to mute security popup about incoming
	// network connections when running locally. When running in prod, App
	// Engine requires that the app listens on the port specified by the
	// environment variable PORT.
	var addr string
	if port := os.Getenv("PORT"); port != "" {
		addr = fmt.Sprintf(":%s", port)
	} else {
		addr = "localhost:8080"
	}

	log.Printf("Listening on addr %s", addr)
	log.Fatal(http.ListenAndServe(addr, mw(mux)))
}
