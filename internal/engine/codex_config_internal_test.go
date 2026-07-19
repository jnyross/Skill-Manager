package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestReinstateCodexConfigEntriesMissingFile(t *testing.T) {
	codexHome := t.TempDir()
	entry := RemovedConfigEntry{
		Offset: 0,
		Raw:    "[[skills.config]]\npath = \"/x/SKILL.md\"\nenabled = false\n",
	}

	if err := reinstateCodexConfigEntries(codexHome, []RemovedConfigEntry{entry}); err != nil {
		t.Fatalf("reinstate with missing config.toml: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(codexHome, "config.toml"))
	if err != nil {
		t.Fatalf("read config after reinstate: %v", err)
	}
	if string(got) != entry.Raw {
		t.Fatalf("config = %q, want %q", string(got), entry.Raw)
	}
}

func TestReinstateCodexConfigEntriesPreservesExistingContent(t *testing.T) {
	codexHome := t.TempDir()
	configPath := filepath.Join(codexHome, "config.toml")
	before := "[profile]\nname = \"default\"\n"
	if err := os.WriteFile(configPath, []byte(before), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	entry := RemovedConfigEntry{
		Offset: 0,
		Raw:    "[[skills.config]]\nname = \"skill\"\nenabled = false\n",
	}
	if err := reinstateCodexConfigEntries(codexHome, []RemovedConfigEntry{entry}); err != nil {
		t.Fatalf("reinstate: %v", err)
	}

	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config after reinstate: %v", err)
	}
	want := entry.Raw + before
	if string(got) != want {
		t.Fatalf("config = %q, want %q", string(got), want)
	}
}

func TestSkillsConfigBlockSpansToleratesWhitespaceAndComments(t *testing.T) {
	content := "[[ skills.config ]]\npath = \"a\"\nenabled = false\n\n" +
		"[[skills.config]] # disable b\npath = \"b\"\nenabled = false\n\n" +
		"[[skills . config]]\npath = \"c\"\nenabled = false\n"

	spans := skillsConfigBlockSpans(content)
	if len(spans) != 3 {
		t.Fatalf("got %d spans, want 3: %+v", len(spans), spans)
	}

	for i, span := range spans {
		var block codexConfig
		raw := content[span.start:span.end]
		if _, err := toml.Decode(raw, &block); err != nil {
			t.Fatalf("span %d: decode %q: %v", i, raw, err)
		}
		if len(block.Skills.Config) != 1 {
			t.Fatalf("span %d: decoded %d config entries, want 1", i, len(block.Skills.Config))
		}
	}
}
