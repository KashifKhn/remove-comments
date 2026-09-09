package languages

import (
	"fmt"
	"sort"
	"strings"

	"github.com/KashifKhn/remove-comments/cli/internal/dart"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/bash"
	"github.com/smacker/go-tree-sitter/c"
	"github.com/smacker/go-tree-sitter/cpp"
	"github.com/smacker/go-tree-sitter/css"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/html"
	"github.com/smacker/go-tree-sitter/java"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/lua"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/rust"
	"github.com/smacker/go-tree-sitter/toml"
	"github.com/smacker/go-tree-sitter/typescript/tsx"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
	"github.com/smacker/go-tree-sitter/yaml"
)

type LangConfig struct {
	Name     string
	Query    string
	Language func() *sitter.Language
}

var byExtension = map[string]LangConfig{
	".js": {
		Name:     "javascript",
		Query:    "(comment) @comment",
		Language: javascript.GetLanguage,
	},
	".mjs": {
		Name:     "javascript",
		Query:    "(comment) @comment",
		Language: javascript.GetLanguage,
	},
	".cjs": {
		Name:     "javascript",
		Query:    "(comment) @comment",
		Language: javascript.GetLanguage,
	},
	".jsx": {
		Name:     "javascript",
		Query:    "(comment) @comment",
		Language: javascript.GetLanguage,
	},
	".ts": {
		Name:     "typescript",
		Query:    "(comment) @comment",
		Language: typescript.GetLanguage,
	},
	".tsx": {
		Name:     "tsx",
		Query:    "(comment) @comment",
		Language: tsx.GetLanguage,
	},
	".lua": {
		Name:     "lua",
		Query:    "(comment) @comment",
		Language: lua.GetLanguage,
	},
	".py": {
		Name:     "python",
		Query:    "(comment) @comment",
		Language: python.GetLanguage,
	},
	".go": {
		Name:     "go",
		Query:    "(comment) @comment",
		Language: golang.GetLanguage,
	},
	".java": {
		Name:     "java",
		Query:    "(line_comment) @comment (block_comment) @comment",
		Language: java.GetLanguage,
	},
	".c": {
		Name:     "c",
		Query:    "(comment) @comment",
		Language: c.GetLanguage,
	},
	".h": {
		Name:     "c",
		Query:    "(comment) @comment",
		Language: c.GetLanguage,
	},
	".cpp": {
		Name:     "cpp",
		Query:    "(comment) @comment",
		Language: cpp.GetLanguage,
	},
	".cc": {
		Name:     "cpp",
		Query:    "(comment) @comment",
		Language: cpp.GetLanguage,
	},
	".cxx": {
		Name:     "cpp",
		Query:    "(comment) @comment",
		Language: cpp.GetLanguage,
	},
	".hpp": {
		Name:     "cpp",
		Query:    "(comment) @comment",
		Language: cpp.GetLanguage,
	},
	".rs": {
		Name:     "rust",
		Query:    "(line_comment) @comment",
		Language: rust.GetLanguage,
	},
	".html": {
		Name:     "html",
		Query:    "(comment) @comment",
		Language: html.GetLanguage,
	},
	".htm": {
		Name:     "html",
		Query:    "(comment) @comment",
		Language: html.GetLanguage,
	},
	".css": {
		Name:     "css",
		Query:    "(comment) @comment",
		Language: css.GetLanguage,
	},
	".yaml": {
		Name:     "yaml",
		Query:    "(comment) @comment",
		Language: yaml.GetLanguage,
	},
	".yml": {
		Name:     "yaml",
		Query:    "(comment) @comment",
		Language: yaml.GetLanguage,
	},
	".toml": {
		Name:     "toml",
		Query:    "(comment) @comment",
		Language: toml.GetLanguage,
	},
	".sh": {
		Name:     "bash",
		Query:    "(comment) @comment",
		Language: bash.GetLanguage,
	},
	".bash": {
		Name:     "bash",
		Query:    "(comment) @comment",
		Language: bash.GetLanguage,
	},
	".dart": {
		Name:     "dart",
		Query:    "(comment) @comment (documentation_comment) @comment",
		Language: dart.GetLanguage,
	},
}

func Get(ext string) (LangConfig, bool) {
	cfg, ok := byExtension[ext]
	return cfg, ok
}

func Supported() []string {
	exts := make([]string, 0, len(byExtension))
	for ext := range byExtension {
		exts = append(exts, ext)
	}
	return exts
}

func Names() []string {
	seen := map[string]bool{}
	names := make([]string, 0, len(byExtension))
	for _, cfg := range byExtension {
		if !seen[cfg.Name] {
			seen[cfg.Name] = true
			names = append(names, cfg.Name)
		}
	}
	sort.Strings(names)
	return names
}

func ValidName(name string) bool {
	for _, cfg := range byExtension {
		if cfg.Name == name {
			return true
		}
	}
	return false
}

func ExtensionsFor(names []string) []string {
	if len(names) == 0 {
		return Supported()
	}
	set := map[string]bool{}
	for _, n := range names {
		for ext, cfg := range byExtension {
			if cfg.Name == n {
				set[ext] = true
			}
		}
	}
	exts := make([]string, 0, len(set))
	for ext := range set {
		exts = append(exts, ext)
	}
	return exts
}

type LangFilter struct {
	names map[string]bool
}

func NewLangFilter(langs []string) *LangFilter {
	if len(langs) == 0 {
		return &LangFilter{}
	}
	names := map[string]bool{}
	for _, l := range langs {
		names[strings.ToLower(strings.TrimSpace(l))] = true
	}
	return &LangFilter{names: names}
}

func (f *LangFilter) Names() []string {
	if len(f.names) == 0 {
		return nil
	}
	out := make([]string, 0, len(f.names))
	for n := range f.names {
		out = append(out, n)
	}
	return out
}

func (f *LangFilter) Allowed(name string) bool {
	if len(f.names) == 0 {
		return true
	}
	return f.names[name]
}

func (f *LangFilter) Validate() error {
	for name := range f.names {
		if !ValidName(name) {
			return fmt.Errorf("unknown language %q (see --list-langs)", name)
		}
	}
	return nil
}
