// Package frontmatter handles reading and writing bean markdown files
// while preserving unknown frontmatter fields like sync state.
package frontmatter

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// BeanFile represents a parsed bean markdown file.
type BeanFile struct {
	Frontmatter map[string]interface{}
	Body        string
	FilePath    string
}

// Read parses a bean markdown file preserving all frontmatter fields.
func Read(filePath string) (*BeanFile, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	return Parse(data, filePath)
}

// Parse parses bean markdown content.
func Parse(data []byte, filePath string) (*BeanFile, error) {
	content := string(data)

	// Check for frontmatter delimiter
	if !strings.HasPrefix(content, "---\n") {
		return &BeanFile{
			Frontmatter: make(map[string]interface{}),
			Body:        content,
			FilePath:    filePath,
		}, nil
	}

	// Find the closing delimiter
	rest := content[4:] // Skip opening "---\n"
	endIdx := strings.Index(rest, "\n---\n")
	if endIdx == -1 {
		// Try "---\r\n" for Windows
		endIdx = strings.Index(rest, "\n---\r\n")
	}
	if endIdx == -1 {
		// No closing delimiter, treat entire content as body
		return &BeanFile{
			Frontmatter: make(map[string]interface{}),
			Body:        content,
			FilePath:    filePath,
		}, nil
	}

	frontmatterYAML := rest[:endIdx]
	body := rest[endIdx+5:] // Skip "\n---\n"

	var frontmatter map[string]interface{}
	if err := yaml.Unmarshal([]byte(frontmatterYAML), &frontmatter); err != nil {
		return nil, fmt.Errorf("parsing frontmatter: %w", err)
	}

	if frontmatter == nil {
		frontmatter = make(map[string]interface{})
	}

	return &BeanFile{
		Frontmatter: frontmatter,
		Body:        body,
		FilePath:    filePath,
	}, nil
}

// Write writes the bean file back to disk, preserving frontmatter structure.
func (bf *BeanFile) Write() error {
	if bf.FilePath == "" {
		return fmt.Errorf("no file path set")
	}

	return bf.WriteTo(bf.FilePath)
}

// WriteTo writes the bean file to the specified path.
func (bf *BeanFile) WriteTo(filePath string) error {
	var buf bytes.Buffer

	// Write frontmatter
	if len(bf.Frontmatter) > 0 {
		buf.WriteString("---\n")

		// Use yaml.v3 encoder for consistent output
		enc := yaml.NewEncoder(&buf)
		enc.SetIndent(2)
		if err := enc.Encode(bf.Frontmatter); err != nil {
			return fmt.Errorf("encoding frontmatter: %w", err)
		}
		_ = enc.Close()

		buf.WriteString("---\n")
	}

	// Write body
	buf.WriteString(bf.Body)

	return os.WriteFile(filePath, buf.Bytes(), 0644)
}

// GetClickUpTaskID returns the ClickUp task ID from frontmatter.
func (bf *BeanFile) GetClickUpTaskID() *string {
	sync, ok := bf.Frontmatter["sync"].(map[string]interface{})
	if !ok {
		return nil
	}
	clickup, ok := sync["clickup"].(map[string]interface{})
	if !ok {
		return nil
	}
	taskID, ok := clickup["task_id"].(string)
	if !ok || taskID == "" {
		return nil
	}
	return &taskID
}

// GetClickUpSyncedAt returns the ClickUp sync timestamp from frontmatter.
func (bf *BeanFile) GetClickUpSyncedAt() *time.Time {
	sync, ok := bf.Frontmatter["sync"].(map[string]interface{})
	if !ok {
		return nil
	}
	clickup, ok := sync["clickup"].(map[string]interface{})
	if !ok {
		return nil
	}
	syncedAt, ok := clickup["synced_at"].(string)
	if !ok || syncedAt == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, syncedAt)
	if err != nil {
		return nil
	}
	return &t
}

// SetClickUpTaskID sets the ClickUp task ID in frontmatter.
func (bf *BeanFile) SetClickUpTaskID(taskID string) {
	bf.ensureClickUpSync()
	sync := bf.Frontmatter["sync"].(map[string]interface{})
	clickup := sync["clickup"].(map[string]interface{})
	clickup["task_id"] = taskID
}

// SetClickUpSyncedAt sets the ClickUp sync timestamp in frontmatter.
func (bf *BeanFile) SetClickUpSyncedAt(t time.Time) {
	bf.ensureClickUpSync()
	sync := bf.Frontmatter["sync"].(map[string]interface{})
	clickup := sync["clickup"].(map[string]interface{})
	clickup["synced_at"] = t.UTC().Format(time.RFC3339)
}

// ClearClickUpSync removes all ClickUp sync data from frontmatter.
func (bf *BeanFile) ClearClickUpSync() {
	sync, ok := bf.Frontmatter["sync"].(map[string]interface{})
	if !ok {
		return
	}
	delete(sync, "clickup")
	// Remove sync entirely if empty
	if len(sync) == 0 {
		delete(bf.Frontmatter, "sync")
	}
}

// ensureClickUpSync ensures the sync.clickup nested structure exists.
func (bf *BeanFile) ensureClickUpSync() {
	if bf.Frontmatter == nil {
		bf.Frontmatter = make(map[string]interface{})
	}

	sync, ok := bf.Frontmatter["sync"].(map[string]interface{})
	if !ok {
		sync = make(map[string]interface{})
		bf.Frontmatter["sync"] = sync
	}

	if _, ok := sync["clickup"].(map[string]interface{}); !ok {
		sync["clickup"] = make(map[string]interface{})
	}
}

// ReadLines reads a file and returns lines, useful for debugging.
func ReadLines(filePath string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}
