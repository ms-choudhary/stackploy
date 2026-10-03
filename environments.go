package main

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Environment struct {
	ID         int64
	Name       string
	DockerHost string
}

func listEnvironments(db *sql.DB) ([]Environment, error) {
	rows, err := db.Query(`SELECT id, name, docker_host FROM environments ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var envs []Environment
	for rows.Next() {
		var e Environment
		if err := rows.Scan(&e.ID, &e.Name, &e.DockerHost); err != nil {
			return nil, err
		}
		envs = append(envs, e)
	}
	return envs, rows.Err()
}

func environmentRoutes(mux *http.ServeMux, db *sql.DB) {
	mux.HandleFunc("GET /environments", func(w http.ResponseWriter, r *http.Request) {
		envs, err := listEnvironments(db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		render(w, r, EnvironmentsPage(envs))
	})

	mux.HandleFunc("POST /environments", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSpace(r.FormValue("name"))
		host := strings.TrimSpace(r.FormValue("docker_host"))
		if name == "" || host == "" {
			http.Error(w, "name and docker host are required", http.StatusBadRequest)
			return
		}
		if _, err := db.Exec(`INSERT INTO environments (name, docker_host) VALUES (?, ?)`, name, host); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Redirect(w, r, "/environments", http.StatusSeeOther)
	})

	mux.HandleFunc("POST /environments/{id}/test", func(w http.ResponseWriter, r *http.Request) {
		var host string
		if err := db.QueryRow(`SELECT docker_host FROM environments WHERE id = ?`, r.PathValue("id")).Scan(&host); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		out, err := docker(ctx, host, nil, "", "version", "--format", "{{.Server.Version}} {{.Server.Os}}/{{.Server.Arch}}")
		render(w, r, TestResult(out, err == nil))
	})
}

// docker runs the docker CLI against host with extra env vars, returning combined output.
func docker(ctx context.Context, host string, extraEnv []string, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), "DOCKER_HOST="+host), extraEnv...)
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		out = []byte(err.Error())
	}
	return strings.TrimSpace(string(out)), err
}
