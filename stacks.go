package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Stack names double as compose project names and directory names.
var validName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

type Stack struct {
	ID          int64
	Name        string
	Environment string
	RepoURL     string
	Branch      string
	ComposePath string
	Status      string
	Output      string
	CreatedAt   time.Time
}

const stackQuery = `SELECT s.id, s.name, e.name, s.repo_url, s.branch, s.compose_path, s.status, s.output, s.created_at
	FROM stacks s JOIN environments e ON e.id = s.environment_id`

func scanStack(row interface{ Scan(...any) error }) (Stack, error) {
	var s Stack
	err := row.Scan(&s.ID, &s.Name, &s.Environment, &s.RepoURL, &s.Branch, &s.ComposePath, &s.Status, &s.Output, &s.CreatedAt)
	return s, err
}

func stackRoutes(mux *http.ServeMux, db *sql.DB, dataDir string) {
	mux.HandleFunc("GET /stacks", func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query(stackQuery + ` ORDER BY s.created_at DESC`)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		var stacks []Stack
		for rows.Next() {
			s, err := scanStack(rows)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			stacks = append(stacks, s)
		}
		render(w, r, StacksPage(stacks))
	})

	mux.HandleFunc("GET /stacks/{id}", func(w http.ResponseWriter, r *http.Request) {
		s, err := scanStack(db.QueryRow(stackQuery+` WHERE s.id = ?`, r.PathValue("id")))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		render(w, r, StackPage(s))
	})

	mux.HandleFunc("GET /stacks/new", func(w http.ResponseWriter, r *http.Request) {
		envs, err := listEnvironments(db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		render(w, r, NewStackPage(envs))
	})

	mux.HandleFunc("GET /stacks/env-row", func(w http.ResponseWriter, r *http.Request) {
		render(w, r, EnvRow())
	})

	mux.HandleFunc("POST /stacks", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s := Stack{
			Name:        strings.TrimSpace(r.FormValue("name")),
			RepoURL:     strings.TrimSpace(r.FormValue("repo_url")),
			Branch:      strings.TrimSpace(r.FormValue("branch")),
			ComposePath: strings.TrimSpace(r.FormValue("compose_path")),
		}
		if !validName.MatchString(s.Name) || s.RepoURL == "" || s.Branch == "" || s.ComposePath == "" {
			http.Error(w, "invalid input: name must be lowercase letters, digits, - or _; all fields are required", http.StatusBadRequest)
			return
		}
		keys, values := r.Form["env_key"], r.Form["env_value"]
		if len(keys) != len(values) {
			http.Error(w, "every env key needs a value field", http.StatusBadRequest)
			return
		}
		envVars := map[string]string{}
		for i, k := range keys {
			if k = strings.TrimSpace(k); k != "" {
				envVars[k] = values[i]
			}
		}
		envJSON, _ := json.Marshal(envVars)

		var dockerHost string
		envID := r.FormValue("environment_id")
		if err := db.QueryRow(`SELECT name, docker_host FROM environments WHERE id = ?`, envID).Scan(&s.Environment, &dockerHost); err != nil {
			http.Error(w, "unknown environment", http.StatusBadRequest)
			return
		}
		res, err := db.Exec(`INSERT INTO stacks (name, environment_id, repo_url, branch, compose_path, env_json) VALUES (?, ?, ?, ?, ?, ?)`,
			s.Name, envID, s.RepoURL, s.Branch, s.ComposePath, string(envJSON))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.ID, _ = res.LastInsertId()

		// Not tied to the request: a closed browser tab shouldn't abort a half-done deploy.
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		s.Output, err = deploy(ctx, s, dockerHost, envVars, filepath.Join(dataDir, "stacks", s.Name))
		s.Status = "deployed"
		if err != nil {
			s.Status = "failed"
		}
		if _, err := db.Exec(`UPDATE stacks SET status = ?, output = ? WHERE id = ?`, s.Status, s.Output, s.ID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		render(w, r, StackPage(s))
	})
}

// deploy clones the repo into dir and runs docker compose up against dockerHost.
func deploy(ctx context.Context, s Stack, dockerHost string, envVars map[string]string, dir string) (string, error) {
	var log strings.Builder
	if err := os.RemoveAll(dir); err != nil {
		return err.Error(), err
	}
	clone := exec.CommandContext(ctx, "git", "clone", "--depth", "1", "--branch", s.Branch, s.RepoURL, dir)
	clone.Env = os.Environ()
	// Pass the token as config via env, so it stays out of argv and the clone's .git/config.
	if token := os.Getenv("GITHUB_TOKEN"); token != "" && strings.HasPrefix(s.RepoURL, "https://github.com/") {
		auth := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
		clone.Env = append(clone.Env,
			"GIT_CONFIG_COUNT=1",
			"GIT_CONFIG_KEY_0=http.https://github.com/.extraHeader",
			"GIT_CONFIG_VALUE_0=Authorization: Basic "+auth,
		)
	}
	out, err := clone.CombinedOutput()
	fmt.Fprintf(&log, "$ git clone %s (%s)\n%s\n", s.RepoURL, s.Branch, out)
	if err != nil {
		return log.String(), err
	}

	var env []string
	for k, v := range envVars {
		env = append(env, k+"="+v)
	}
	fmt.Fprintf(&log, "$ docker compose -p %s -f %s up -d\n", s.Name, s.ComposePath)
	out2, err := docker(ctx, dockerHost, env, dir, "compose", "-p", s.Name, "-f", s.ComposePath, "up", "-d")
	log.WriteString(out2)
	return log.String(), err
}
