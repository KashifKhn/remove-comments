package cmd

import (
	"testing"

	"github.com/KashifKhn/remove-comments/cli/internal/config"
	"github.com/KashifKhn/remove-comments/cli/internal/languages"
)

func TestParseLangs_FlagWins(t *testing.T) {
	got := parseLangs("go,java", []string{"python"})
	if len(got) != 2 || got[0] != "go" || got[1] != "java" {
		t.Errorf("parseLangs = %v, want [go java]", got)
	}
}

func TestParseLangs_ConfigFallback(t *testing.T) {
	got := parseLangs("", []string{"python"})
	if len(got) != 1 || got[0] != "python" {
		t.Errorf("parseLangs = %v, want [python]", got)
	}
}

func TestParseLangs_None(t *testing.T) {
	if got := parseLangs("", nil); got != nil {
		t.Errorf("parseLangs = %v, want nil", got)
	}
}

func TestMergeStrings_FlagAppendsToConfig(t *testing.T) {
	got := mergeStrings([]string{"*.g.dart"}, []string{"vendor/**"})
	if len(got) != 2 || got[0] != "*.g.dart" || got[1] != "vendor/**" {
		t.Errorf("mergeStrings = %v, want [*.g.dart vendor/**]", got)
	}
}

func TestMergeStrings_Dedup(t *testing.T) {
	got := mergeStrings([]string{"a", "b"}, []string{"b", "c"})
	if len(got) != 3 {
		t.Errorf("mergeStrings = %v, want 3 unique values", got)
	}
}

func TestMergeStrings_EmptyConfig(t *testing.T) {
	got := mergeStrings(nil, []string{"x"})
	if len(got) != 1 || got[0] != "x" {
		t.Errorf("mergeStrings = %v, want [x]", got)
	}
}

func TestLoadConfig_NoConfigFlag(t *testing.T) {
	flagNoConfig = true
	defer func() { flagNoConfig = false }()

	cfg, err := loadConfig(".")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Exclude) != 0 {
		t.Errorf("expected empty config, got %+v", cfg)
	}
}

func TestLanguagesValidate(t *testing.T) {
	if err := languages.NewLangFilter([]string{"go", "java", "typescript"}).Validate(); err != nil {
		t.Errorf("valid langs rejected: %v", err)
	}
	if err := languages.NewLangFilter([]string{"klingon"}).Validate(); err == nil {
		t.Error("invalid lang should be rejected")
	}
}

func TestSkipGeneratedResolution(t *testing.T) {
	skip := !flagNoSkipGenerated
	if !skip {
		t.Error("default should skip generated files")
	}
}

func TestConfigTypes(t *testing.T) {
	tr := true
	cfg := config.Config{SkipGenerated: &tr}
	if cfg.SkipGenerated == nil || !*cfg.SkipGenerated {
		t.Error("SkipGenerated pointer roundtrip failed")
	}
}
