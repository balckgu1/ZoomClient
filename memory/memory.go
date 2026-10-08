package memory

const (
	// MaxBodyChars 是 search_memory 返回的单条正文上限（15KB）。
	// 超出时截断并提示模型改用 read_file 读取完整 memory 文件。
	MaxBodyChars = 15 * 1024
	// MaxMemoryEntries 是 system prompt 中最多显示的 memory 条目数。
	MaxMemoryEntries = 50
)

const (
	MTypeUser      = "user"
	MTypeFeedback  = "feedback"
	MTypeProject   = "project"
	MTypeReference = "reference"
)

// MemoryPriority 定义type优先级，数值越小优先级越高
var MemoryPriority = map[string]int{
	MTypeFeedback:  0,
	MTypeUser:      1,
	MTypeProject:   2,
	MTypeReference: 3,
}

type MemoryDocument struct {
	FrontMatter MemoryFrontMatter
	Body        string
}

type MemoryFrontMatter struct {
	Name        string
	Description string
	Type        string
}
