// Package config loads and validates the YAML configuration.
package config

import (
	"fmt"
	"os"
	"regexp"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Interval  time.Duration `yaml:"interval"`
	StateFile string        `yaml:"state_file"`
	Listen    string        `yaml:"listen"`
	Notifiers []Notifier    `yaml:"notifiers"`
	Providers []Provider    `yaml:"providers"`
}

type Notifier struct {
	Name   string `yaml:"name"`    // unique; keys per-notifier delivery state
	Type   string `yaml:"type"`    // "telegram" | "slack"
	Token  string `yaml:"token"`   // telegram bot token | slack bot token (xoxb-…)
	ChatID string `yaml:"chat_id"` // telegram chat | slack channel id (C…)
}

type Provider struct {
	Name    string   `yaml:"name"`
	Sources []Source `yaml:"sources"`
}

// Source describes one data source. Fields used depend on Type.
type Source struct {
	Name     string        `yaml:"name"`
	Type     string        `yaml:"type"` // "webpage" | "github_releases"
	URL      string        `yaml:"url"`
	Interval time.Duration `yaml:"interval"` // 0 → Config.Interval
	Template string        `yaml:"template"` // html/template, data = source.Item ([]source.Item when Value is set)

	// webpage
	Items  string            `yaml:"items"`  // XPath; one Item per matched node, Key = normalize-space(node)
	Fields map[string]string `yaml:"fields"` // name → XPath; "./…" relative to item, else document-wide zipped by index; "link" → Item.Link
	Key    string            `yaml:"key"`    // XPath relative to item; overrides Key. Node-sets are joined with " · "
	Value  string            `yaml:"value"`  // XPath relative to item; set → report changes of Value per Key, one digest per check

	// github_releases
	Filter string `yaml:"filter"` // regexp over title+content; empty → every entry
}

// envRef matches only ${VAR}, so a bare $ in templates or regexes stays literal.
var envRef = regexp.MustCompile(`\$\{\w+\}`)

// Load reads path, expands ${ENV} references, applies defaults and validates.
func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(envRef.ReplaceAllFunc(b, func(m []byte) []byte { return []byte(os.Getenv(string(m[2 : len(m)-1]))) }), &c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if c.Interval == 0 {
		c.Interval = 30 * time.Minute
	}
	if c.StateFile == "" {
		c.StateFile = "state.json"
	}
	if c.Listen == "" {
		c.Listen = ":8080"
	}
	return &c, c.validate()
}

func (c *Config) validate() error {
	if len(c.Notifiers) == 0 {
		return fmt.Errorf("no notifiers configured")
	}
	names := map[string]bool{}
	for _, n := range c.Notifiers {
		if n.Name == "" || names[n.Name] {
			return fmt.Errorf("notifier %q: name must be set and unique", n.Name)
		}
		names[n.Name] = true
		if n.Type != "telegram" && n.Type != "slack" {
			return fmt.Errorf("notifier %q: unknown type %q", n.Name, n.Type)
		}
		if n.Token == "" || n.ChatID == "" {
			return fmt.Errorf("notifier %q: token and chat_id required", n.Name)
		}
	}
	for _, p := range c.Providers {
		for _, s := range p.Sources {
			if s.Name == "" || s.URL == "" || s.Template == "" {
				return fmt.Errorf("provider %q: source needs name, url and template", p.Name)
			}
			switch s.Type {
			case "webpage":
				if s.Items == "" {
					return fmt.Errorf("source %s/%s: items xpath required", p.Name, s.Name)
				}
			case "github_releases":
				if s.Key != "" || s.Value != "" {
					return fmt.Errorf("source %s/%s: key/value apply to webpage only", p.Name, s.Name)
				}
			default:
				return fmt.Errorf("source %s/%s: unknown type %q", p.Name, s.Name, s.Type)
			}
		}
	}
	return nil
}
