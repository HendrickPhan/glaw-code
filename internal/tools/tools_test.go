package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBashSuccess(t *testing.T) {
	r := NewRegistry(t.TempDir())
	out, err := r.ExecuteTool(context.Background(), "bash", json.RawMessage(`{"command":"echo hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	if !strings.Contains(out.Content, "hello") {
		t.Errorf("output = %q, want to contain 'hello'", out.Content)
	}
}

func TestBashFailure(t *testing.T) {
	r := NewRegistry(t.TempDir())
	out, err := r.ExecuteTool(context.Background(), "bash", json.RawMessage(`{"command":"exit 1"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for failing command")
	}
}

func TestBashTimeout(t *testing.T) {
	r := NewRegistry(t.TempDir())
	out, err := r.ExecuteTool(context.Background(), "bash", json.RawMessage(`{"command":"sleep 10","timeout":100}`))
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected timeout error")
	}
	if !strings.Contains(out.Content, "timed out") {
		t.Errorf("output = %q, want 'timed out'", out.Content)
	}
}

func TestReadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewRegistry(dir)
	out, err := r.ExecuteTool(context.Background(), "read_file", json.RawMessage(`{"path":"test.txt"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	if out.Content != "hello world" {
		t.Errorf("Content = %q, want %q", out.Content, "hello world")
	}
}

func TestReadFileNotFound(t *testing.T) {
	r := NewRegistry(t.TempDir())
	out, err := r.ExecuteTool(context.Background(), "read_file", json.RawMessage(`{"path":"nonexistent.txt"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for missing file")
	}
}

func TestWriteFile(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry(dir)

	out, err := r.ExecuteTool(context.Background(), "write_file", json.RawMessage(`{"path":"sub/dir/test.txt","content":"hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}

	data, err := os.ReadFile(filepath.Join(dir, "sub", "dir", "test.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Errorf("file content = %q, want %q", string(data), "hello")
	}
}

func TestWriteFileOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewRegistry(dir)
	out, err := r.ExecuteTool(context.Background(), "write_file", json.RawMessage(`{"path":"test.txt","content":"new"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "new" {
		t.Errorf("content = %q, want %q", string(data), "new")
	}
}

func TestEditFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewRegistry(dir)
	out, err := r.ExecuteTool(context.Background(), "edit_file", json.RawMessage(`{"path":"test.txt","old_string":"world","new_string":"Go"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "hello Go" {
		t.Errorf("content = %q, want %q", string(data), "hello Go")
	}
}

func TestEditFileNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewRegistry(dir)
	out, err := r.ExecuteTool(context.Background(), "edit_file", json.RawMessage(`{"path":"test.txt","old_string":"missing","new_string":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error when old_string not found")
	}
}

func TestEditFileMultipleOccurrences(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("aaa bbb aaa"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewRegistry(dir)
	out, err := r.ExecuteTool(context.Background(), "edit_file", json.RawMessage(`{"path":"test.txt","old_string":"aaa","new_string":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error when old_string found multiple times")
	}
}

func TestGlobSearch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "c.txt"), []byte("text"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewRegistry(dir)
	out, err := r.ExecuteTool(context.Background(), "glob_search", json.RawMessage(`{"pattern":"*.go"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	if !strings.Contains(out.Content, "a.go") || !strings.Contains(out.Content, "b.go") {
		t.Errorf("output = %q, want a.go and b.go", out.Content)
	}
	if strings.Contains(out.Content, "c.txt") {
		t.Error("should not match c.txt")
	}
}

func TestGlobSearchNoMatch(t *testing.T) {
	r := NewRegistry(t.TempDir())
	out, err := r.ExecuteTool(context.Background(), "glob_search", json.RawMessage(`{"pattern":"*.xyz"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	if !strings.Contains(out.Content, "No files") {
		t.Errorf("output = %q, want 'No files'", out.Content)
	}
}

func TestGrepSearch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\nfunc hello() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.go"), []byte("package main\nfunc world() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewRegistry(dir)
	out, err := r.ExecuteTool(context.Background(), "grep_search", json.RawMessage(`{"pattern":"func hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	if !strings.Contains(out.Content, "hello") {
		t.Errorf("output = %q, want to contain 'hello'", out.Content)
	}
	if strings.Contains(out.Content, "world") {
		t.Error("should not match world")
	}
}

func TestGrepSearchNoMatch(t *testing.T) {
	r := NewRegistry(t.TempDir())
	if err := os.WriteFile(filepath.Join(t.TempDir(), "test.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := r.ExecuteTool(context.Background(), "grep_search", json.RawMessage(`{"pattern":"xyz123"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	if !strings.Contains(out.Content, "No matches") {
		t.Errorf("output = %q, want 'No matches'", out.Content)
	}
}

func TestExecuteToolUnknown(t *testing.T) {
	r := NewRegistry(t.TempDir())
	out, err := r.ExecuteTool(context.Background(), "nonexistent", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for unknown tool")
	}
}

// --- Analyze Tool Tests ---

// createMultiLangProject creates a temporary project with Go, Python, JavaScript, and TypeScript files.
func createMultiLangProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Helper to create directories and files; fatal on error.
	mkdir := func(path string) {
		t.Helper()
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFile := func(path string, data []byte) {
		t.Helper()
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Go project files
	mkdir(filepath.Join(dir, "cmd", "server"))
	mkdir(filepath.Join(dir, "internal", "api"))
	writeFile(filepath.Join(dir, "go.mod"), []byte("module github.com/test/project\n\ngo 1.22\n"))
	writeFile(filepath.Join(dir, "cmd", "server", "main.go"), []byte(`package main

import (
	"fmt"
	"github.com/test/project/internal/api"
)

func main() {
	fmt.Println("hello")
}
`))
	writeFile(filepath.Join(dir, "internal", "api", "server.go"), []byte(`package api

import "net/http"

// Server represents an HTTP server.
type Server struct{}

func NewServer() *Server { return &Server{} }
`))
	writeFile(filepath.Join(dir, "internal", "api", "server_test.go"), []byte(`package api

import "testing"

func TestNewServer(t *testing.T) {}
`))

	// Python project files
	mkdir(filepath.Join(dir, "src", "myapp"))
	writeFile(filepath.Join(dir, "src", "myapp", "__init__.py"), []byte(`"""My application package."""

import os
import sys

def main():
    print("hello from python")
`))
	writeFile(filepath.Join(dir, "src", "myapp", "utils.py"), []byte(`"""Utility functions."""

import json
import re

def helper(x):
    return x.strip()
`))
	writeFile(filepath.Join(dir, "src", "myapp", "test_utils.py"), []byte(`"""Tests for utils."""

import unittest

class TestUtils(unittest.TestCase):
    def test_helper(self):
        self.assertEqual(helper("  hi  "), "hi")
`))
	writeFile(filepath.Join(dir, "requirements.txt"), []byte("flask==2.0\nrequests==2.28\n"))
	writeFile(filepath.Join(dir, "pyproject.toml"), []byte(`[project]
name = "myapp"
version = "0.1.0"
`))

	// JavaScript / TypeScript project files
	mkdir(filepath.Join(dir, "web", "src"))
	writeFile(filepath.Join(dir, "package.json"), []byte(`{"name": "myapp-web", "dependencies": {"express": "^4.18"}}}
`))
	writeFile(filepath.Join(dir, "web", "src", "index.js"), []byte(`const express = require('express');

function handler(req, res) {
    res.send('hello');
}
`))
	writeFile(filepath.Join(dir, "web", "src", "app.ts"), []byte(`import express from 'express';

interface App {
    port: number;
}

function createApp(config: App) {
    return config;
}
`))
	writeFile(filepath.Join(dir, "web", "src", "app.test.ts"), []byte(`import { createApp } from './app';

describe('createApp', () => {
    it('works', () => {
        expect(createApp({ port: 3000 })).toBeDefined();
    });
});
`))

	// Docs & config
	writeFile(filepath.Join(dir, "README.md"), []byte(`# Test Project
Multi-language test project.
`))
	writeFile(filepath.Join(dir, "Makefile"), []byte(`build:\n\tgo build ./...\n`))
	writeFile(filepath.Join(dir, "Dockerfile"), []byte(`FROM golang:1.22\nCOPY . .\n`))

	// Create .git to test exclusion
	mkdir(filepath.Join(dir, ".git", "objects"))
	writeFile(filepath.Join(dir, ".git", "HEAD"), []byte("ref: refs/heads/main"))

	// Create node_modules to test exclusion
	mkdir(filepath.Join(dir, "node_modules", "express"))
	writeFile(filepath.Join(dir, "node_modules", "express", "index.js"), []byte("module.exports = {};"))

	return dir
}

func TestAnalyzeToolFullMode(t *testing.T) {
	dir := createMultiLangProject(t)
	r := NewRegistry(dir)

	out, err := r.ExecuteTool(context.Background(), "analyze", json.RawMessage(`{"mode":"full"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}

	// Verify key sections
	for _, section := range []string{"PROJECT ANALYSIS REPORT", "Structure", "Lines of Code",
		"File Type Distribution", "Infrastructure", "Modules", "Dependencies"} {
		if !strings.Contains(out.Content, section) {
			t.Errorf("missing section %q in full analysis output", section)
		}
	}

	// Verify multiple languages detected
	for _, lang := range []string{"go", "python", "javascript", "typescript"} {
		if !strings.Contains(out.Content, lang) {
			t.Errorf("should detect %s language", lang)
		}
	}

	// Verify .glaw/analysis.json saved
	analysisPath := filepath.Join(dir, ".glaw", "analysis.json")
	if _, err := os.Stat(analysisPath); os.IsNotExist(err) {
		t.Error(".glaw/analysis.json should exist after full analysis")
	}
}

func TestAnalyzeToolSummaryMode(t *testing.T) {
	dir := createMultiLangProject(t)
	r := NewRegistry(dir)

	// Run full first to create cache
	_, _ = r.ExecuteTool(context.Background(), "analyze", json.RawMessage(`{"mode":"full"}`))

	// Now run summary — should load from cache
	out, err := r.ExecuteTool(context.Background(), "analyze", json.RawMessage(`{"mode":"summary"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	if !strings.Contains(out.Content, "PROJECT ANALYSIS REPORT") {
		t.Error("summary should contain report header")
	}
	if !strings.Contains(out.Content, "cached from") {
		t.Error("summary should indicate it came from cache")
	}
}

func TestAnalyzeToolGraphMode(t *testing.T) {
	dir := createMultiLangProject(t)
	r := NewRegistry(dir)

	out, err := r.ExecuteTool(context.Background(), "analyze", json.RawMessage(`{"mode":"graph","format":"mermaid"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	if !strings.Contains(out.Content, "graph TD") {
		t.Error("mermaid graph should contain 'graph TD'")
	}

	// Test DOT format
	out, err = r.ExecuteTool(context.Background(), "analyze", json.RawMessage(`{"mode":"graph","format":"dot"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	if !strings.Contains(out.Content, "digraph") {
		t.Error("DOT graph should contain 'digraph'")
	}

	// Test JSON adjacency format
	out, err = r.ExecuteTool(context.Background(), "analyze", json.RawMessage(`{"mode":"graph","format":"json"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	var adj map[string][]string
	if err := json.Unmarshal([]byte(out.Content), &adj); err != nil {
		t.Errorf("JSON adjacency parse error: %v", err)
	}
}

func TestAnalyzeToolInvalidMode(t *testing.T) {
	r := NewRegistry(t.TempDir())
	out, err := r.ExecuteTool(context.Background(), "analyze", json.RawMessage(`{"mode":"invalid"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for invalid mode")
	}
}

func TestAnalyzeDetectsMultipleLanguages(t *testing.T) {
	dir := createMultiLangProject(t)
	r := NewRegistry(dir)

	out, err := r.ExecuteTool(context.Background(), "analyze", json.RawMessage(`{"mode":"full"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}

	// Verify infrastructure flags
	if !strings.Contains(out.Content, "go.mod") {
		t.Error("should detect go.mod")
	}
	if !strings.Contains(out.Content, "package.json") {
		t.Error("should detect package.json")
	}
	if !strings.Contains(out.Content, "pyproject.toml") {
		t.Error("should detect pyproject.toml")
	}
	if !strings.Contains(out.Content, "requirements.txt") {
		t.Error("should detect requirements.txt")
	}
	if !strings.Contains(out.Content, "Dockerfile") {
		t.Error("should detect Dockerfile")
	}
	if !strings.Contains(out.Content, "Makefile") {
		t.Error("should detect Makefile")
	}

	// Verify test files counted
	if !strings.Contains(out.Content, "Test Files") {
		t.Error("should show test file count")
	}

	// Verify .git and node_modules excluded
	for _, excluded := range []string{".git/", "node_modules/"} {
		for _, line := range strings.Split(out.Content, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), excluded) {
				t.Errorf("excluded directory %q should not appear in top-level dirs", excluded)
			}
		}
	}
}

func TestGetToolSpecs(t *testing.T) {
	r := NewRegistry(t.TempDir())
	specs := r.GetToolSpecs()
	if len(specs) != 16 {
		t.Errorf("expected 16 tool specs, got %d", len(specs))
	}

	names := make(map[string]bool)
	for _, s := range specs {
		names[s.Name] = true
	}
	for _, name := range []string{"bash", "read_file", "write_file", "edit_file", "glob_search", "grep_search", "web_fetch", "web_search", "todo_write", "tool_search", "notebook_edit", "sleep", "send_user_message", "config", "analyze", "sub_agent"} {
		if !names[name] {
			t.Errorf("missing tool spec: %s", name)
		}
	}
}

func TestWebFetch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Hello from server"))
	}))
	defer ts.Close()

	r := NewRegistry(t.TempDir())
	input, _ := json.Marshal(map[string]string{"url": ts.URL})
	out, err := r.ExecuteTool(context.Background(), "web_fetch", input)
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	if !strings.Contains(out.Content, "Hello from server") {
		t.Errorf("output = %q", out.Content)
	}
}
