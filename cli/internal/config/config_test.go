package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoad_FullConfig(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, FileName, `
exclude:
  - "*.g.dart"
  - "vendor/**"
include:
  - "src/**"
langs:
  - Java
  - TypeScript
skip-generated: true
max-file-size: 2048
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Exclude) != 2 || cfg.Exclude[0] != "*.g.dart" || cfg.Exclude[1] != "vendor/**" {
		t.Errorf("unexpected exclude: %v", cfg.Exclude)
	}
	if len(cfg.Include) != 1 || cfg.Include[0] != "src/**" {
		t.Errorf("unexpected include: %v", cfg.Include)
	}
	if len(cfg.Langs) != 2 || cfg.Langs[0] != "java" || cfg.Langs[1] != "typescript" {
		t.Errorf("unexpected langs: %v", cfg.Langs)
	}
	if cfg.SkipGenerated == nil || !*cfg.SkipGenerated {
		t.Errorf("expected skip-generated: true, got %v", cfg.SkipGenerated)
	}
	if cfg.MaxFileSize != 2048 {
		t.Errorf("expected max-file-size 2048, got %d", cfg.MaxFileSize)
	}
}

func TestLoad_EmptyConfig(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, FileName, "")

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Exclude) != 0 || len(cfg.Include) != 0 || len(cfg.Langs) != 0 {
		t.Errorf("expected empty config, got %+v", cfg)
	}
	if cfg.SkipGenerated != nil {
		t.Errorf("expected nil SkipGenerated, got %v", *cfg.SkipGenerated)
	}
}

func TestLoad_InvalidYaml(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, FileName, "exclude: [unclosed")

	if _, err := Load(path); err == nil {
		t.Error("expected error for invalid yaml")
	}
}

func TestLoad_MissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Error("expected error for missing file")
	}
}

func TestDiscover_Root(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, FileName, "exclude: []\n")
	sub := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}

	got := Discover(sub)
	want := filepath.Join(dir, FileName)
	if got != want {
		t.Errorf("Discover(%q) = %q, want %q", sub, got, want)
	}
}

func TestDiscover_FromFileArg(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, FileName, "exclude: []\n")
	target := filepath.Join(dir, "main.go")
	if err := os.WriteFile(target, []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}

	got := Discover(target)
	want := filepath.Join(dir, FileName)
	if got != want {
		t.Errorf("Discover(%q) = %q, want %q", target, got, want)
	}
}

func TestDiscover_NoneFound(t *testing.T) {
	if got := Discover(t.TempDir()); got != "" {
		t.Errorf("expected empty result, got %q", got)
	}
}

func TestNormalizeLangs(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{"nil", nil, nil},
		{"empty", []string{}, nil},
		{"mixed case and spaces", []string{" Go ", "JAVA", "typescript"}, []string{"go", "java", "typescript"}},
		{"dedup", []string{"go", "Go", "GO"}, []string{"go"}},
		{"empty strings dropped", []string{"", "go"}, []string{"go"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeLangs(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("normalizeLangs(%v) = %v, want %v", tt.input, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("normalizeLangs(%v) = %v, want %v", tt.input, got, tt.want)
					break
				}
			}
		})
	}
}
