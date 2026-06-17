# 运行时模型切换功能设计

## 概述

为 cc-learn 项目新增运行时模型配置与切换能力：
- CLI 斜杠命令 `/setmode`、`/selectmode`、`/models` 管理模型预设
- Web 前端顶部右侧下拉框选择模型
- 模型预设持久化到 `config/models.yaml`，重启后保留

## 数据模型

### Preset（模型预设）

```go
// model/preset.go
type Preset struct {
    Name      string `yaml:"name" json:"name"`
    Type      string `yaml:"type" json:"type"`                        // "openai" | "ollama" | "anthropic" | "gemini"
    BaseURL   string `yaml:"base_url,omitempty" json:"base_url,omitempty"`
    APIKey    string `yaml:"api_key,omitempty" json:"api_key,omitempty"`
    ModelName string `yaml:"model_name" json:"model_name"`
}
```

### Registry（模型注册表）

```go
// model/registry.go
type Registry struct {
    presets  map[string]*Preset
    active   string
    mu       sync.RWMutex
    filepath string
}
```

核心方法：
- `NewRegistry(filepath string)` — 从 models.yaml 加载预设
- `Add(preset *Preset)` — 添加/覆盖预设，持久化
- `Get(name string) *Preset` — 查询预设
- `List() []*Preset` — 列出所有预设
- `Active() *Preset` — 当前激活的预设
- `Select(name string) (*Preset, error)` — 切换激活模型，返回预设
- `Remove(name string) error` — 删除预设，持久化
- `BuildClient(preset *Preset) (clients.ChatClient, string)` — 根据预设创建 ChatClient

## 持久化机制

### 存储文件：`config/models.yaml`

```yaml
models:
  - name: "deepseek-v3"
    type: "openai"
    base_url: "https://api.deepseek.com"
    api_key: "sk-xxx"
    model_name: "deepseek-v4-flash"
```

### 加载策略

1. 启动时 `model.NewRegistry("config/models.yaml")` 读取文件
2. 文件不存在则创建空 Registry（不报错）
3. 将 `config.yaml` 中的 **openai** 和 **ollama** 后端作为默认预设自动注册（如果 models.yaml 中没有同名预设）
4. 初始 active 模型 = 启动时 `-m` 参数指定后端对应的预设

### 写入策略

- `/setmode` 和 `POST /api/models` 添加后立即全量写入
- `DELETE /api/models/{name}` 删除后立即全量写入
- 使用 `gopkg.in/yaml.v3` 序列化

## CLI 斜杠命令

### `/setmode -m <name> -t <type> -u <baseurl> -k <apikey> [--model <modelname>]`

- 解析参数，构造 Preset，调用 `Registry.Add()`
- 未指定 `--model` 时使用 `-m` 的值作为模型名
- 持久化到 `config/models.yaml`
- 输出：`Model preset "deepseek-v3" saved (type: openai, model: deepseek-v4-flash)`

### `/selectmode -m <name>`

- 调用 `Registry.Select(name)`
- 调用 `AgentSession.SwitchModel(name)` 热替换客户端
- 保留对话历史不丢失
- 输出：`Switched to model "deepseek-v3" (deepseek-v4-flash)`

### `/models`

- 列出所有已配置的模型预设，标记当前激活
- 输出示例：`* deepseek-v3 (openai, deepseek-v4-flash) [active]`

### `/help` 更新

新增上述三条命令的说明。

## AgentSession 集成

在 `AgentSession` 中增加 `ModelRegistry *model.Registry` 字段，新增 `SwitchModel` 方法：

```go
func (s *AgentSession) SwitchModel(name string) error {
    preset, err := s.ModelRegistry.Select(name)
    client, modelName := model.BuildClient(preset)
    s.Client = client
    s.ModelName = modelName
    s.CompactManager.UpdateModel(client, modelName)
    s.Pipeline.UpdateModelName(modelName)
    return nil
}
```

切换模型保留 `state.Messages`，对话历史不丢失。

## Web API 端点

| 方法 | 路径 | 功能 |
|------|------|------|
| GET | `/api/models` | 返回所有预设列表 + 当前激活模型名 |
| POST | `/api/models` | 新增预设（body: `{name, type, base_url, api_key, model_name}`） |
| POST | `/api/model/select` | 切换当前模型（body: `{name}`） |
| DELETE | `/api/models/{name}` | 删除指定预设 |

### GET /api/models 响应示例

```json
{
  "models": [
    {"name": "deepseek-v3", "type": "openai", "base_url": "https://api.deepseek.com", "model_name": "deepseek-v4-flash"}
  ],
  "active": "deepseek-v3"
}
```

## 前端 UI

### ModelSelector 组件

- 位置：`app-main` 区域顶部，StatusBar 右侧
- 默认显示当前模型名称，点击展开下拉列表
- 下拉列表显示所有已配置模型，选择后调用 `POST /api/model/select`
- 切换成功后 toast 提示
- 底部可选「+ 添加模型」入口，展开表单（name/type/url/key/model_name）

### 新增前端 API 函数

```typescript
// lib/api.ts
export function fetchModels(): Promise<{models: ModelPreset[], active: string}>
export function addModel(preset: ModelPreset): Promise<{status: string}>
export function selectModel(name: string): Promise<{status: string}>
export function deleteModel(name: string): Promise<{status: string}>
```

## 新增文件清单

| 文件 | 说明 |
|------|------|
| `model/preset.go` | Preset 数据结构 |
| `model/registry.go` | Registry 核心逻辑 + BuildClient |
| `model/registry_test.go` | Registry 单元测试 |
| `web/frontend/src/components/ModelSelector.tsx` | 前端模型选择器组件 |

## 修改文件清单

| 文件 | 修改内容 |
|------|---------|
| `main.go` | AgentSession 增加 ModelRegistry 字段；handleSlashCommand 新增 setmode/selectmode/models 命令；SwitchModel 方法 |
| `web/server.go` | 注册 `/api/models`、`/api/model/select`、`/api/models/` 路由 |
| `web/handlers.go` | 新增 handleModels、handleAddModel、handleSelectModel、handleDeleteModel 处理函数 |
| `web/frontend/src/components/App.tsx` | 集成 ModelSelector 组件，新增相关状态和回调 |
| `web/frontend/src/lib/api.ts` | 新增 fetchModels/addModel/selectModel/deleteModel 函数 |
| `web/frontend/src/types.ts` | 新增 ModelPreset 类型定义 |
| `compact/compact.go` | CompactManager 新增 UpdateModel 方法 |
| `prompt/pipeline.go` | Pipeline 新增 UpdateModelName 方法 |
| `config/config.yaml.example` | 新增 models.yaml 说明 |
