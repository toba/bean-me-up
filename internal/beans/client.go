package beans

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/toba/beans/pkg/client"
)

// ExtensionDataOp is an alias for the beans client package type.
type ExtensionDataOp = client.ExtensionDataOp

// Client executes beans CLI commands and parses output.
type Client struct {
	beansPath string
	gc        *client.Client
}

// NewClient creates a new beans CLI client.
func NewClient(beansPath string) *Client {
	return &Client{
		beansPath: beansPath,
		gc:        client.New(client.WithBeansPath(beansPath)),
	}
}

// List returns all beans from the beans CLI.
func (c *Client) List() ([]Bean, error) {
	args := []string{"list", "--json", "--full"}
	if c.beansPath != "" {
		args = append(args, "--beans-path", c.beansPath)
	}

	out, err := c.exec(args...)
	if err != nil {
		return nil, err
	}

	var beans []Bean
	if err := json.Unmarshal(out, &beans); err != nil {
		return nil, fmt.Errorf("parsing beans JSON: %w", err)
	}

	return beans, nil
}

// Get returns a specific bean by ID.
func (c *Client) Get(id string) (*Bean, error) {
	args := []string{"show", "--json", id}
	if c.beansPath != "" {
		args = append(args, "--beans-path", c.beansPath)
	}

	out, err := c.exec(args...)
	if err != nil {
		return nil, err
	}

	// beans show --json with a single ID returns a single object (not an array)
	var bean Bean
	if err := json.Unmarshal(out, &bean); err != nil {
		return nil, fmt.Errorf("parsing bean JSON: %w", err)
	}

	if bean.ID == "" {
		return nil, fmt.Errorf("bean not found: %s", id)
	}

	return &bean, nil
}

// GetMultiple returns multiple beans by ID.
func (c *Client) GetMultiple(ids []string) ([]Bean, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	// For a single ID, use Get (beans show returns object, not array)
	if len(ids) == 1 {
		bean, err := c.Get(ids[0])
		if err != nil {
			return nil, err
		}
		return []Bean{*bean}, nil
	}

	args := []string{"show", "--json"}
	args = append(args, ids...)
	if c.beansPath != "" {
		args = append(args, "--beans-path", c.beansPath)
	}

	out, err := c.exec(args...)
	if err != nil {
		return nil, err
	}

	var beans []Bean
	if err := json.Unmarshal(out, &beans); err != nil {
		return nil, fmt.Errorf("parsing beans JSON: %w", err)
	}

	return beans, nil
}

// SetExtensionData sets extension data on a single bean.
func (c *Client) SetExtensionData(id, name string, data map[string]any) error {
	return c.gc.SetExtensionData(id, name, data)
}

// RemoveExtensionData removes extension data from a single bean.
func (c *Client) RemoveExtensionData(id, name string) error {
	return c.gc.RemoveExtensionData(id, name)
}

// SetExtensionDataBatch sets extension data on multiple beans in a single
// GraphQL call using aliased mutations.
func (c *Client) SetExtensionDataBatch(ops []ExtensionDataOp) error {
	return c.gc.SetExtensionDataBatch(ops)
}

// exec runs a beans command and returns the output.
func (c *Client) exec(args ...string) ([]byte, error) {
	cmd := exec.Command("beans", args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			return nil, fmt.Errorf("beans %s: %s", strings.Join(args, " "), string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("beans %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}
