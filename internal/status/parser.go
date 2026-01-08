package status

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Status represents parsed status file data
type Status struct {
	Status    string
	TaskID    string
	Updated   int64
	TabName   string
	SessionID string
}

// ParseStatusFile parses a status file
func ParseStatusFile(path string) (*Status, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	status := &Status{}
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "status":
			status.Status = value
		case "task_id":
			status.TaskID = value
		case "updated":
			if ts, err := strconv.ParseInt(value, 10, 64); err == nil {
				status.Updated = ts
			}
		case "tab_name":
			status.TabName = value
		case "session_id":
			status.SessionID = value
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if status.TaskID == "" {
		return nil, fmt.Errorf("missing task_id in status file")
	}

	return status, nil
}

// WriteStatusFile writes a status file
func WriteStatusFile(path string, status *Status) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	lines := []string{
		fmt.Sprintf("status=%s", status.Status),
		fmt.Sprintf("task_id=%s", status.TaskID),
		fmt.Sprintf("updated=%d", status.Updated),
	}

	if status.TabName != "" {
		lines = append(lines, fmt.Sprintf("tab_name=%s", status.TabName))
	}
	if status.SessionID != "" {
		lines = append(lines, fmt.Sprintf("session_id=%s", status.SessionID))
	}

	for _, line := range lines {
		if _, err := file.WriteString(line + "\n"); err != nil {
			return err
		}
	}

	return nil
}
