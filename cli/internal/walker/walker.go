package walker

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/boyter/gocodewalker"

	"github.com/KashifKhn/remove-comments/cli/internal/languages"
)

type FileEntry struct {
	Path string
	Ext  string
	Lang languages.LangConfig
}

type Options struct {
	Langs       []string
	MaxFileSize int64
	Exclude     []string
	Include     []string
}

func (o Options) Filter() *languages.LangFilter {
	return languages.NewLangFilter(o.Langs)
}

func Walk(root string, opts Options) ([]FileEntry, []error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, []error{err}
	}
	if !info.IsDir() {
		return walkSingleFile(root, opts)
	}

	queue := make(chan *gocodewalker.File, 512)

	filter := opts.Filter()
	exts := buildAllowList(filter)

	fw := gocodewalker.NewFileWalker(root, queue)
	fw.AllowListExtensions = exts
	fw.ExcludeDirectory = []string{".git", "node_modules", "vendor", ".idea", ".vscode"}

	var walkErr error
	fw.SetErrorHandler(func(e error) bool {
		walkErr = e
		return true
	})

	go func() {
		_ = fw.Start()
	}()

	var entries []FileEntry
	var errs []error

	for f := range queue {
		rel := relPath(root, f.Location)
		if opts.excluded(f.Location, rel) {
			continue
		}
		if !opts.included(f.Location, rel) {
			continue
		}

		ext := filepath.Ext(f.Filename)
		cfg, ok := languages.Get(ext)
		if !ok {
			continue
		}
		if !filter.Allowed(cfg.Name) {
			continue
		}

		fi, statErr := os.Stat(f.Location)
		if statErr != nil {
			errs = append(errs, statErr)
			continue
		}
		if opts.MaxFileSize > 0 && fi.Size() > opts.MaxFileSize {
			continue
		}

		entries = append(entries, FileEntry{
			Path: f.Location,
			Ext:  ext,
			Lang: cfg,
		})
	}

	if walkErr != nil {
		errs = append(errs, walkErr)
	}

	return entries, errs
}

func walkSingleFile(path string, opts Options) ([]FileEntry, []error) {
	if opts.excluded(path, "") {
		return nil, nil
	}
	if !opts.included(path, "") {
		return nil, nil
	}
	ext := filepath.Ext(path)
	cfg, ok := languages.Get(ext)
	if !ok {
		return nil, nil
	}
	if !opts.Filter().Allowed(cfg.Name) {
		return nil, nil
	}
	fi, err := os.Stat(path)
	if err != nil {
		return nil, []error{err}
	}
	if opts.MaxFileSize > 0 && fi.Size() > opts.MaxFileSize {
		return nil, nil
	}
	return []FileEntry{{Path: path, Ext: ext, Lang: cfg}}, nil
}

func (o Options) excluded(path, rel string) bool {
	return matchAny(o.Exclude, path, rel)
}

func (o Options) included(path, rel string) bool {
	if len(o.Include) == 0 {
		return true
	}
	return matchAny(o.Include, path, rel)
}

func relPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return path
	}
	return rel
}

func matchAny(patterns []string, path, rel string) bool {
	if len(patterns) == 0 {
		return false
	}
	normalized := filepath.ToSlash(path)
	base := filepath.Base(path)
	relNorm := filepath.ToSlash(rel)
	for _, pattern := range patterns {
		p := filepath.ToSlash(pattern)
		if p == "" {
			continue
		}
		if strings.Contains(p, "/") {
			if ok, _ := doublestar.Match(p, normalized); ok {
				return true
			}
			if relNorm != "" && relNorm != normalized {
				if ok, _ := doublestar.Match(p, relNorm); ok {
					return true
				}
			}
		}
		if ok, _ := doublestar.Match(p, base); ok {
			return true
		}
	}
	return false
}

func buildAllowList(filter *languages.LangFilter) []string {
	source := languages.ExtensionsFor(filter.Names())
	exts := make([]string, 0, len(source))
	for _, ext := range source {
		if len(ext) > 1 && ext[0] == '.' {
			exts = append(exts, ext[1:])
		} else {
			exts = append(exts, ext)
		}
	}
	return exts
}
