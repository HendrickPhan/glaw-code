package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"
)

const dataFile = "todo.json"

// Task represents a single todo item.
type Task struct {
	ID        int       `json:"id"`
	Title     string    `json:"title"`
	Done      bool      `json:"done"`
	CreatedAt time.Time `json:"created_at"`
}

// loadTasks reads the tasks from the JSON file. Returns an empty slice if the
// file does not exist or is empty.
func loadTasks() ([]Task, error) {
	data, err := os.ReadFile(dataFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []Task{}, nil
		}
		return nil, fmt.Errorf("reading %s: %w", dataFile, err)
	}

	if len(data) == 0 {
		return []Task{}, nil
	}

	var tasks []Task
	if err := json.Unmarshal(data, &tasks); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", dataFile, err)
	}
	return tasks, nil
}

// saveTasks writes the tasks back to the JSON file with pretty-printing.
func saveTasks(tasks []Task) error {
	data, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling tasks: %w", err)
	}
	return os.WriteFile(dataFile, data, 0644)
}

// nextID returns the next available ID (max existing ID + 1, or 1 if empty).
func nextID(tasks []Task) int {
	maxID := 0
	for _, t := range tasks {
		if t.ID > maxID {
			maxID = t.ID
		}
	}
	return maxID + 1
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "add":
		cmdAdd()
	case "list":
		cmdList()
	case "done":
		cmdDone()
	case "delete":
		cmdDelete()
	default:
		fmt.Printf("Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`Usage:
  todo add "Buy groceries"  - Add a new task
  todo list                  - Show all tasks with status
  todo done <id>             - Mark task as completed
  todo delete <id>           - Remove a task`)
}

// cmdAdd adds a new task with the given title.
func cmdAdd() {
	if len(os.Args) < 3 {
		fmt.Println("Error: task title is required.")
		fmt.Println(`Usage: todo add "Buy groceries"`)
		os.Exit(1)
	}

	title := os.Args[2]

	tasks, err := loadTasks()
	if err != nil {
		fmt.Printf("Error loading tasks: %v\n", err)
		os.Exit(1)
	}

	task := Task{
		ID:        nextID(tasks),
		Title:     title,
		Done:      false,
		CreatedAt: time.Now(),
	}

	tasks = append(tasks, task)

	if err := saveTasks(tasks); err != nil {
		fmt.Printf("Error saving tasks: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Task added: #%d %s\n", task.ID, task.Title)
}

// cmdList displays all tasks with their status.
func cmdList() {
	tasks, err := loadTasks()
	if err != nil {
		fmt.Printf("Error loading tasks: %v\n", err)
		os.Exit(1)
	}

	if len(tasks) == 0 {
		fmt.Println("No tasks yet. Add one with: todo add \"Your task\"")
		return
	}

	fmt.Println("Your Tasks:")
	fmt.Println("─────────────────────────────────────────────")
	for _, t := range tasks {
		status := "✗"
		if t.Done {
			status = "✓"
		}
		fmt.Printf("  %s #%d  %s\n", status, t.ID, t.Title)
	}
	fmt.Println("─────────────────────────────────────────────")

	pending := 0
	completed := 0
	for _, t := range tasks {
		if t.Done {
			completed++
		} else {
			pending++
		}
	}
	fmt.Printf("  %d pending, %d completed\n", pending, completed)
}

// cmdDone marks a task as completed by its ID.
func cmdDone() {
	if len(os.Args) < 3 {
		fmt.Println("Error: task ID is required.")
		fmt.Println("Usage: todo done <id>")
		os.Exit(1)
	}

	id, err := strconv.Atoi(os.Args[2])
	if err != nil {
		fmt.Printf("Error: invalid task ID %q (must be a number)\n", os.Args[2])
		os.Exit(1)
	}

	tasks, err := loadTasks()
	if err != nil {
		fmt.Printf("Error loading tasks: %v\n", err)
		os.Exit(1)
	}

	found := false
	for i, t := range tasks {
		if t.ID == id {
			if t.Done {
				fmt.Printf("Task #%d is already completed.\n", id)
				return
			}
			tasks[i].Done = true
			found = true
			break
		}
	}

	if !found {
		fmt.Printf("Error: task #%d not found.\n", id)
		os.Exit(1)
	}

	if err := saveTasks(tasks); err != nil {
		fmt.Printf("Error saving tasks: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Task #%d marked as done.\n", id)
}

// cmdDelete removes a task by its ID.
func cmdDelete() {
	if len(os.Args) < 3 {
		fmt.Println("Error: task ID is required.")
		fmt.Println("Usage: todo delete <id>")
		os.Exit(1)
	}

	id, err := strconv.Atoi(os.Args[2])
	if err != nil {
		fmt.Printf("Error: invalid task ID %q (must be a number)\n", os.Args[2])
		os.Exit(1)
	}

	tasks, err := loadTasks()
	if err != nil {
		fmt.Printf("Error loading tasks: %v\n", err)
		os.Exit(1)
	}

	found := false
	for i, t := range tasks {
		if t.ID == id {
			tasks = append(tasks[:i], tasks[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		fmt.Printf("Error: task #%d not found.\n", id)
		os.Exit(1)
	}

	if err := saveTasks(tasks); err != nil {
		fmt.Printf("Error saving tasks: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Task #%d deleted.\n", id)
}
