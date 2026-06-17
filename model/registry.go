package model

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"zoomClient/clients"

	"gopkg.in/yaml.v3"
)

// Registry 管理模型预设的注册、查询、切换和持久化
type Registry struct {
	presets  map[string]*Preset
	active   string
	mu       sync.RWMutex
	filepath string
}

// modelsFile 持久化文件的顶层结构
type modelsFile struct {
	Models []*Preset `yaml:"models"`
}

// NewRegistry 从指定文件加载预设，文件不存在时返回空 Registry
func NewRegistry(filepath string) *Registry {
	r := &Registry{
		presets:  make(map[string]*Preset),
		filepath: filepath,
	}
	r.load()
	return r
}

// Add 添加或覆盖预设，并持久化到文件
func (r *Registry) Add(p *Preset) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.presets[p.Name] = p
	r.save()
}

// Get 查询指定名称的预设，不存在返回 nil
func (r *Registry) Get(name string) *Preset {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.presets[name]
}

// List 返回所有预设，按名称排序
func (r *Registry) List() []*Preset {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]*Preset, 0, len(r.presets))
	for _, p := range r.presets {
		list = append(list, p)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	return list
}

// Active 返回当前激活的预设名称
func (r *Registry) Active() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.active
}

// ActivePreset 返回当前激活的预设对象
func (r *Registry) ActivePreset() *Preset {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.presets[r.active]
}

// Select 切换激活模型，返回切换后的预设
func (r *Registry) Select(name string) (*Preset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.presets[name]
	if !ok {
		return nil, fmt.Errorf("model preset %q not found", name)
	}
	r.active = name
	return p, nil
}

// SetActive 直接设置激活模型名（不校验），用于启动时设置
func (r *Registry) SetActive(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.active = name
}

// Remove 删除预设并持久化
func (r *Registry) Remove(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.presets[name]; !ok {
		return fmt.Errorf("model preset %q not found", name)
	}
	delete(r.presets, name)
	if r.active == name {
		r.active = ""
	}
	r.save()
	return nil
}

// RegisterDefault 注册默认预设（仅当同名预设不存在时）
func (r *Registry) RegisterDefault(p *Preset) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.presets[p.Name]; !exists {
		r.presets[p.Name] = p
	}
}

// load 从文件加载预设
func (r *Registry) load() {
	data, err := os.ReadFile(r.filepath)
	if err != nil {
		// 文件不存在或无法读取，使用空 Registry
		return
	}
	var mf modelsFile
	if err := yaml.Unmarshal(data, &mf); err != nil {
		return
	}
	for _, p := range mf.Models {
		if p.Name != "" {
			r.presets[p.Name] = p
		}
	}
}

// save 将所有预设全量写入文件
func (r *Registry) save() {
	// 调用方已持有锁
	mf := modelsFile{Models: make([]*Preset, 0, len(r.presets))}
	for _, p := range r.presets {
		mf.Models = append(mf.Models, p)
	}
	sort.Slice(mf.Models, func(i, j int) bool { return mf.Models[i].Name < mf.Models[j].Name })
	data, err := yaml.Marshal(&mf)
	if err != nil {
		return
	}
	_ = os.WriteFile(r.filepath, data, 0644)
}

// BuildClient 根据预设创建对应的 ChatClient 和模型名
func BuildClient(p *Preset) (clients.ChatClient, string) {
	switch strings.ToLower(p.Type) {
	case "openai":
		baseURL := p.BaseURL
		if baseURL == "" {
			baseURL = "https://api.openai.com"
		}
		return clients.NewOpenAIClient(baseURL, p.APIKey), p.ModelName
	case "ollama":
		baseURL := p.BaseURL
		if baseURL == "" {
			baseURL = "http://127.0.0.1:11434"
		}
		return clients.NewOllamaClient(baseURL), p.ModelName
	case "anthropic":
		return clients.NewAnthropicClient(p.APIKey), p.ModelName
	case "gemini":
		return clients.NewGeminiClient(p.APIKey), p.ModelName
	default:
		// 默认按 openai 兼容处理
		return clients.NewOpenAIClient(p.BaseURL, p.APIKey), p.ModelName
	}
}
