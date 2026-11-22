#!/bin/sh
set -eu

root=$(mktemp -d)
cat >"$root/jj.toml" <<'EOF'
[user]
name = "demo"
email = "demo@example.com"

[ui]
paginate = "never"

[operation]
hostname = "demo"
username = "demo"
EOF
export JJ_CONFIG="$root/jj.toml"

config=${XDG_CONFIG_HOME:-$HOME/.config}
mkdir -p "$root/config/jjr"
for entry in "$config"/*; do
	if [ -e "$entry" ] && [ "$(basename "$entry")" != jjr ]; then
		ln -s "$entry" "$root/config/"
	fi
done
cp "${JJR_DEMO_THEME:-docs/demo/dark.toml}" "$root/config/jjr/config.toml"

mkdir -p "$root/repo/server" "$root/repo/cmd/shortener"
cd "$root/repo"
jj git init --quiet

cat >go.mod <<'EOF'
module example.com/shortener

go 1.27
EOF

cat >README.md <<'EOF'
# shortener

A tiny URL shortener.

## API

- `GET /{code}` redirects to the stored URL.
- `POST /` stores a URL under a code.
EOF

cat >cmd/shortener/main.go <<'EOF'
package main

import (
	"log"
	"net/http"

	"example.com/shortener/server"
)

func main() {
	h := server.New(server.NewMemory())
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{code}", h.Resolve)
	mux.HandleFunc("POST /", h.Create)
	log.Fatal(http.ListenAndServe(":8080", mux))
}
EOF

cat >server/handler.go <<'EOF'
package server

import (
	"encoding/json"
	"net/http"
)

type Store interface {
	Get(code string) (string, bool)
	Put(code, url string) error
}

type Handler struct {
	store Store
}

func New(store Store) *Handler {
	return &Handler{store: store}
}

func (h *Handler) Resolve(w http.ResponseWriter, r *http.Request) {
	url, ok := h.store.Get(r.PathValue("code"))
	if !ok {
		http.Error(w, "not found", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, url, http.StatusFound)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
		URL  string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.store.Put(req.Code, req.URL); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}
EOF

cat >server/memory.go <<'EOF'
package server

import "sync"

type Memory struct {
	mu    sync.RWMutex
	links map[string]string
}

func NewMemory() *Memory {
	return &Memory{links: map[string]string{}}
}

func (m *Memory) Get(code string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	url, ok := m.links[code]
	return url, ok
}

func (m *Memory) Put(code, url string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.links[code] = url
	return nil
}
EOF

jj describe --quiet -m "feat: add url shortener"
jj bookmark create --quiet main -r @
jj new --quiet
git init --quiet --bare "$root/origin.git"
jj git remote add origin "$root/origin.git"
jj bookmark track --quiet main --remote origin
jj git push --quiet --bookmark main

git clone --quiet "$root/origin.git" "$root/upstream"
mkdir -p "$root/upstream/.github/workflows"
cat >"$root/upstream/.github/workflows/ci.yml" <<'EOF'
name: CI
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
      - uses: actions/setup-go@v6
      - run: go test ./...
EOF
git -C "$root/upstream" add .
git -C "$root/upstream" -c user.name=demo -c user.email=demo@example.com commit --quiet -m "ci: run the tests"
git -C "$root/upstream" push --quiet

cat >>README.md <<'EOF'

`POST /` answers `400 Bad Request` when the URL is missing.
EOF
jj describe --quiet -m "docs: document the missing url error"
jj new --quiet

cat >.gitignore <<'EOF'
/shortener
EOF
jj describe --quiet -m "chore: ignore the binary"
jj new --quiet

cat >Makefile <<'EOF'
build:
	go build ./cmd/shortener
EOF
jj describe --quiet -m "build: add a makefile"
jj new --quiet

cat >.editorconfig <<'EOF'
root = true

[*.go]
indent_style = tab
EOF
jj describe --quiet -m "chore: add an editorconfig"
jj new --quiet

cat >cmd/shortener/main.go <<'EOF'
package main

import (
	"log"
	"net/http"
	"time"

	"example.com/shortener/server"
)

func main() {
	h := server.New(server.NewMemory())
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{code}", h.Resolve)
	mux.HandleFunc("POST /", h.Create)
	srv := &http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}
EOF

cat >server/handler.go <<'EOF'
package server

import (
	"encoding/json"
	"net/http"
)

type Store interface {
	Get(code string) (string, bool)
	Put(code, url string) error
}

type Handler struct {
	store Store
}

func New(store Store) *Handler {
	return &Handler{store: store}
}

func (h *Handler) Resolve(w http.ResponseWriter, r *http.Request) {
	url, ok := h.store.Get(r.PathValue("code"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, url, http.StatusFound)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
		URL  string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.URL == "" {
		http.Error(w, "url is required", http.StatusBadRequest)
		return
	}
	if err := h.store.Put(req.Code, req.URL); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}
EOF

cat >server/handler_test.go <<'EOF'
package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveUnknownCode(t *testing.T) {
	h := New(NewMemory())
	req := httptest.NewRequest(http.MethodGet, "/nope", nil)
	req.SetPathValue("code", "nope")
	rec := httptest.NewRecorder()

	h.Resolve(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
EOF

jj describe --quiet -m "wip"
echo "$root"
