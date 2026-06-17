package model

import (
	"os"
	"path/filepath"
	"testing"
)

func tempFile(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "models.yaml")
}

func TestNewRegistry_FileNotExist(t *testing.T) {
	r := NewRegistry(filepath.Join(t.TempDir(), "nonexistent.yaml"))
	if len(r.List()) != 0 {
		t.Fatalf("expected empty registry, got %d presets", len(r.List()))
	}
}

func TestAddAndList(t *testing.T) {
	r := NewRegistry(tempFile(t))
	r.Add(&Preset{Name: "gpt4o", Type: "openai", BaseURL: "https://api.openai.com", APIKey: "sk-test", ModelName: "gpt-4o"})
	r.Add(&Preset{Name: "deepseek", Type: "openai", BaseURL: "https://api.deepseek.com", APIKey: "sk-ds", ModelName: "deepseek-v3"})

	list := r.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 presets, got %d", len(list))
	}
	// List is sorted by name
	if list[0].Name != "deepseek" || list[1].Name != "gpt4o" {
		t.Fatalf("unexpected sort order: %v", list)
	}
}

func TestAddPersistence(t *testing.T) {
	fp := tempFile(t)
	r1 := NewRegistry(fp)
	r1.Add(&Preset{Name: "test", Type: "openai", BaseURL: "http://localhost", APIKey: "k", ModelName: "m"})

	// Reload from file
	r2 := NewRegistry(fp)
	if r2.Get("test") == nil {
		t.Fatal("expected preset to persist across reload")
	}
	if r2.Get("test").ModelName != "m" {
		t.Fatalf("expected model_name 'm', got %q", r2.Get("test").ModelName)
	}
}

func TestSelect(t *testing.T) {
	r := NewRegistry(tempFile(t))
	r.Add(&Preset{Name: "a", Type: "openai", ModelName: "a-model"})
	r.Add(&Preset{Name: "b", Type: "ollama", ModelName: "b-model"})

	p, err := r.Select("b")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name != "b" || r.Active() != "b" {
		t.Fatalf("expected active='b', got %q", r.Active())
	}

	_, err = r.Select("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent preset")
	}
}

func TestRemove(t *testing.T) {
	fp := tempFile(t)
	r := NewRegistry(fp)
	r.Add(&Preset{Name: "x", Type: "openai", ModelName: "x-model"})
	r.SetActive("x")

	if err := r.Remove("x"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Get("x") != nil {
		t.Fatal("expected preset to be removed")
	}
	if r.Active() != "" {
		t.Fatalf("expected active cleared, got %q", r.Active())
	}

	// Verify persistence
	r2 := NewRegistry(fp)
	if r2.Get("x") != nil {
		t.Fatal("expected removal to persist")
	}
}

func TestRemoveNonexistent(t *testing.T) {
	r := NewRegistry(tempFile(t))
	if err := r.Remove("nope"); err == nil {
		t.Fatal("expected error for removing nonexistent preset")
	}
}

func TestRegisterDefault(t *testing.T) {
	r := NewRegistry(tempFile(t))
	r.Add(&Preset{Name: "openai", Type: "openai", APIKey: "custom", ModelName: "custom-model"})

	// RegisterDefault should not overwrite
	r.RegisterDefault(&Preset{Name: "openai", Type: "openai", APIKey: "default", ModelName: "default-model"})
	if r.Get("openai").APIKey != "custom" {
		t.Fatal("RegisterDefault should not overwrite existing preset")
	}

	// RegisterDefault should add new
	r.RegisterDefault(&Preset{Name: "ollama", Type: "ollama", ModelName: "llama"})
	if r.Get("ollama") == nil {
		t.Fatal("RegisterDefault should add new preset")
	}
}

func TestBuildClient(t *testing.T) {
	tests := []struct {
		presetType string
		expectNil  bool
	}{
		{"openai", false},
		{"ollama", false},
		{"anthropic", false},
		{"gemini", false},
		{"unknown", false}, // defaults to openai
	}
	for _, tt := range tests {
		p := &Preset{Name: "test", Type: tt.presetType, BaseURL: "http://localhost:1234", APIKey: "k", ModelName: "m"}
		client, modelName := BuildClient(p)
		if client == nil {
			t.Fatalf("BuildClient(%s) returned nil client", tt.presetType)
		}
		if modelName != "m" {
			t.Fatalf("BuildClient(%s) returned wrong model: %s", tt.presetType, modelName)
		}
	}
}

func TestSaveAndLoad(t *testing.T) {
	fp := tempFile(t)
	r := NewRegistry(fp)
	r.Add(&Preset{Name: "a", Type: "openai", BaseURL: "http://a", APIKey: "ka", ModelName: "ma"})
	r.Add(&Preset{Name: "b", Type: "ollama", BaseURL: "http://b", ModelName: "mb"})

	// Verify file exists
	if _, err := os.Stat(fp); err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}

	r2 := NewRegistry(fp)
	if len(r2.List()) != 2 {
		t.Fatalf("expected 2 presets after reload, got %d", len(r2.List()))
	}
}
