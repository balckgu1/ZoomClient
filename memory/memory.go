package memory

const (
	MaxBodyPreviewChars = 500
	MaxMemoryEntries    = 50 // system prompt 中最多显示的 memory 条目数
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

type MemoryFrontMatter struct {
	Name        string
	Description string
	Type        string
}

type MemoryDocument struct {
	FrontMatter MemoryFrontMatter
	Body        string
}
