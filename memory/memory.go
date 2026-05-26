package memory

const (
	MTypeUser      = "user"
	MTypeFeedback  = "feedback"
	MTypeProject   = "project"
	MTypeReference = "reference"
)

type MemoryFrontMatter struct {
	Name        string
	Description string
	Type        string
}

type MemoryDocument struct {
	FrontMatter MemoryFrontMatter
	Body        string
}
