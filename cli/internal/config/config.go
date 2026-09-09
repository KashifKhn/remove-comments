package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Exclude       []string `yaml:"exclude"`
	Include       []string `yaml:"include"`
	Langs         []string `yaml:"langs"`
	SkipGenerated *bool    `yaml:"skip-generated"`
	MaxFileSize   int64    `yaml:"max-file-size"`
}

const FileName = ".remove-comments.yaml"

func Load(path string) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse %s: %w", path, err)
	}
	cfg.Langs = normalizeLangs(cfg.Langs)
	return cfg, nil
}

func Discover(root string) string {
	abs, err := filepath.Abs(root)
	if err != nil {
		return ""
	}
	if info, err := os.Stat(abs); err == nil && !info.IsDir() {
		abs = filepath.Dir(abs)
	}
	for {
		candidate := filepath.Join(abs, FileName)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return ""
		}
		abs = parent
	}
}

func normalizeLangs(langs []string) []string {
	if len(langs) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(langs))
	for _, l := range langs {
		l = strings.ToLower(strings.TrimSpace(l))
		if l == "" || seen[l] {
			continue
		}
		seen[l] = true
		out = append(out, l)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
