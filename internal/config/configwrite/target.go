package configwrite

// TargetPath is the config file that runtime config mutations persist to.
// It is resolved once at startup (see configfx) from the same locations the
// config is read from, and injected into components that write config at
// runtime (the LLM registry, the config mutation applier), so every writer
// agrees on a single target file.
type TargetPath string
