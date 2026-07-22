// Package configwrite persists individual sections of a YAML config file
// without disturbing the rest of it.
//
// It exists so that more than one component (the LLM registry, the config
// mutation API) can write to config.yml with the same guarantees, implemented
// and tested in one place:
//
//   - Atomic: the new content is written to a temp file in the same directory,
//     fsync'd, then renamed over the target, so a crash mid-write leaves either
//     the complete old file or the complete new one — never a truncated file.
//   - Non-destructive: only the addressed section is replaced; every other
//     section, and the comments and key ordering within them, survives, because
//     the file is edited as a yaml.Node tree rather than round-tripped through a
//     map.
//   - Fail-closed: a present-but-unreadable or unparseable file aborts the write
//     and is left untouched, rather than being overwritten with just the new
//     section.
package configwrite

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const defaultMode fs.FileMode = 0o644

// WriteSection sets the value at keyPath in the YAML file at path, preserving
// everything else, and writes the result atomically. keyPath addresses a nested
// mapping key, e.g. []string{"classifier", "llm"} for classifier.llm or
// []string{"tmdb"} for a top-level section. An empty keyPath is an error.
//
// A missing file is created containing only the addressed section. A file that
// cannot be read or parsed aborts the write and is left untouched.
func WriteSection(path string, keyPath []string, value any) error {
	if len(keyPath) == 0 {
		return errors.New("configwrite: empty key path")
	}

	root, mode, err := loadTree(path)
	if err != nil {
		return err
	}

	var valueNode yaml.Node
	if encErr := valueNode.Encode(value); encErr != nil {
		return fmt.Errorf("configwrite: encode value: %w", encErr)
	}

	// Descend to (creating as needed) the parent mapping of the final key, then
	// set the final key to the encoded value.
	parent := topMapping(root)
	for _, k := range keyPath[:len(keyPath)-1] {
		parent = childMapping(parent, k)
	}

	setMapChild(parent, keyPath[len(keyPath)-1], &valueNode)

	out, marshalErr := yaml.Marshal(root)
	if marshalErr != nil {
		return fmt.Errorf("configwrite: marshal config: %w", marshalErr)
	}

	if writeErr := atomicWriteFile(path, out, mode); writeErr != nil {
		return fmt.Errorf("configwrite: write config: %w", writeErr)
	}

	return nil
}

// loadTree reads path into a yaml document node and returns the mode to
// preserve on rewrite. A missing file yields an empty document and the default
// mode. A present-but-unreadable or unparseable file is a hard error: we must
// not overwrite a file we could not fully understand.
func loadTree(path string) (*yaml.Node, fs.FileMode, error) {
	info, statErr := os.Stat(path)

	switch {
	case statErr == nil:
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, 0, fmt.Errorf("configwrite: read config: %w", readErr)
		}

		root := &yaml.Node{}
		if unmarshalErr := yaml.Unmarshal(data, root); unmarshalErr != nil {
			return nil, 0, fmt.Errorf("configwrite: parse config: %w", unmarshalErr)
		}

		return root, info.Mode().Perm(), nil
	case errors.Is(statErr, fs.ErrNotExist):
		return &yaml.Node{}, defaultMode, nil
	default:
		return nil, 0, fmt.Errorf("configwrite: stat config: %w", statErr)
	}
}

// topMapping returns the top-level mapping node of a config document,
// initializing the document and mapping when the source file was empty or held
// something other than a mapping at the root.
func topMapping(root *yaml.Node) *yaml.Node {
	if root.Kind == 0 {
		root.Kind = yaml.DocumentNode
	}

	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		root.Content = []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}
	}

	return root.Content[0]
}

// childMapping returns the mapping node stored under key in m, creating an
// empty mapping (and the key) when it is absent or not itself a mapping. A
// non-mapping existing value is replaced rather than merged, since there is no
// structure to preserve in that case.
func childMapping(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			if m.Content[i+1].Kind == yaml.MappingNode {
				return m.Content[i+1]
			}

			child := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			m.Content[i+1] = child

			return child
		}
	}

	child := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	setMapChild(m, key, child)

	return child
}

// setMapChild sets key=val in mapping m, replacing an existing value in place
// (preserving its position) or appending a new key/value pair.
func setMapChild(m *yaml.Node, key string, val *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1] = val
			return
		}
	}

	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		val,
	)
}

// atomicWriteFile writes data to path atomically: a temp file in the same
// directory (so the rename cannot cross filesystems), fsync'd and closed, then
// renamed over the target. mode is applied to the temp file before the rename.
func atomicWriteFile(path string, data []byte, mode fs.FileMode) error {
	dir := filepath.Dir(path)

	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}

	tmpName := tmp.Name()
	// Best-effort cleanup: harmless no-op after a successful rename.
	defer func() { _ = os.Remove(tmpName) }()

	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}

	if err = tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}

	if err = tmp.Close(); err != nil {
		return err
	}

	if err = os.Chmod(tmpName, mode); err != nil {
		return err
	}

	return os.Rename(tmpName, path)
}
