package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func getFreePort(t *testing.T) string {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("Failed to get free port: %v", err)
	}
	defer l.Close()
	return fmt.Sprintf("%d", l.Addr().(*net.TCPAddr).Port)
}

func startGitDB(t *testing.T, port string, root string) *exec.Cmd {
	cmd := exec.Command("go", "run", "main.go", "--root", root, "--port", port)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("Failed to get stdout pipe: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("Failed to get stderr pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start GitDB: %v", err)
	}

	go io.Copy(io.Discard, stdout)
	go io.Copy(io.Discard, stderr)

	url := "http://localhost:" + port + "/sql"
	for i := 0; i < 20; i++ {
		resp, err := http.Post(url, "application/json", bytes.NewBuffer([]byte(`{"sql":"SELECT * FROM nothing"}`)))
		if err == nil {
			_ = resp.Body.Close()
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	return cmd
}

func stopGitDB(t *testing.T, cmd *exec.Cmd) {
    if cmd != nil && cmd.Process != nil {
        _ = cmd.Process.Kill()
    }
}

func runSQL(t *testing.T, url, sessionID, sql string) any {
	body, _ := json.Marshal(map[string]string{"sql": sql})

	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Session-ID", sessionID)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("HTTP POST failed: %v", err)
	}
	defer resp.Body.Close()

	var result any
	data, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to parse JSON: %v\nRaw: %s", err, string(data))
	}
	return result
}

func TestCreateAndQuery(t *testing.T) {
	port := getFreePort(t)
	root := t.TempDir()
	server := startGitDB(t, port, root)
	defer stopGitDB(t, server)

	url := "http://localhost:" + port + "/sql"
	sessionID := "test-session"

	runSQL(t, url, sessionID, "CREATE DATABASE testdb")
	runSQL(t, url, sessionID, "USE DATABASE testdb")
	runSQL(t, url, sessionID, "CREATE TABLE users (name STRING, email STRING)")

	res1 := runSQL(t, url, sessionID, "INSERT INTO users (name, email) VALUES ('Alice', 'alice@example.com')")
	t.Logf("Insert Alice result: %+v", res1)

	res2 := runSQL(t, url, sessionID, "INSERT INTO users (name, email) VALUES ('Bob', 'bob@example.com')")
	t.Logf("Insert Bob result: %+v", res2)

	files, _ := filepath.Glob(filepath.Join(root, "testdb", "users", "*.json"))
	for _, f := range files {
		t.Logf("User table file: %s", filepath.Base(f))
	}

	result := runSQL(t, url, sessionID, "SELECT * FROM users")
	t.Logf("Query result: %+v", result)

	outer, ok := result.([]any)
	if !ok || len(outer) == 0 {
		t.Fatalf("Unexpected or empty response: %#v", result)
	}

	rows, ok := outer[0].([]any)
	if !ok {
		t.Fatalf("Unexpected inner result format: %#v", outer[0])
	}

	if len(rows) != 2 {
		t.Fatalf("Expected 2 rows, got %d", len(rows))
	}
}
