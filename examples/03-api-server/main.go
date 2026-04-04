package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Item represents a single item in the data store.
type Item struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateItemRequest represents the JSON body for creating a new item.
type CreateItemRequest struct {
	Name string `json:"name"`
}

// ErrorResponse represents a standard error response.
type ErrorResponse struct {
	Error string `json:"error"`
}

// Store is an in-memory data store for items.
type Store struct {
	mu      sync.RWMutex
	items   map[int]Item
	nextID  int
}

// NewStore creates a new empty Store.
func NewStore() *Store {
	return &Store{
		items:  make(map[int]Item),
		nextID: 1,
	}
}

// Create adds a new item to the store and returns it.
func (s *Store) Create(name string) Item {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := Item{
		ID:        s.nextID,
		Name:      name,
		CreatedAt: time.Now(),
	}
	s.items[s.nextID] = item
	s.nextID++

	return item
}

// GetAll returns all items in the store.
func (s *Store) GetAll() []Item {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]Item, 0, len(s.items))
	for _, item := range s.items {
		items = append(items, item)
	}
	return items
}

// GetByID returns a single item by its ID, or an error if not found.
func (s *Store) GetByID(id int) (Item, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	item, ok := s.items[id]
	return item, ok
}

// Delete removes an item by its ID and returns whether it existed.
func (s *Store) Delete(id int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, ok := s.items[id]
	if ok {
		delete(s.items, id)
	}
	return ok
}

var store = NewStore()

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/items", itemsHandler)
	mux.HandleFunc("/items/", itemHandler)

	addr := ":8080"
	fmt.Printf("API server listening on %s\n", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

// itemsHandler handles requests to /items (GET and POST).
func itemsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handleListItems(w, r)
	case http.MethodPost:
		handleCreateItem(w, r)
	default:
		methodNotAllowed(w, "GET, POST")
	}
}

// itemHandler handles requests to /items/{id} (GET and DELETE).
func itemHandler(w http.ResponseWriter, r *http.Request) {
	// Extract the ID from the URL path
	path := strings.TrimPrefix(r.URL.Path, "/items/")
	if path == "" {
		http.NotFound(w, r)
		return
	}

	id, err := strconv.Atoi(path)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid item ID"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		handleGetItem(w, r, id)
	case http.MethodDelete:
		handleDeleteItem(w, r, id)
	default:
		methodNotAllowed(w, "GET, DELETE")
	}
}

// handleListItems returns all items.
func handleListItems(w http.ResponseWriter, r *http.Request) {
	items := store.GetAll()
	writeJSON(w, http.StatusOK, items)
}

// handleCreateItem creates a new item from the JSON body.
func handleCreateItem(w http.ResponseWriter, r *http.Request) {
	var req CreateItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid JSON body"})
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "name is required"})
		return
	}

	item := store.Create(req.Name)
	writeJSON(w, http.StatusCreated, item)
}

// handleGetItem returns a single item by ID.
func handleGetItem(w http.ResponseWriter, r *http.Request, id int) {
	item, ok := store.GetByID(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "item not found"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

// handleDeleteItem deletes an item by ID.
func handleDeleteItem(w http.ResponseWriter, r *http.Request, id int) {
	deleted := store.Delete(id)
	if !deleted {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "item not found"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}

// methodNotAllowed writes a 405 Method Not Allowed response.
func methodNotAllowed(w http.ResponseWriter, allowed string) {
	w.Header().Set("Allow", allowed)
	writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
}
