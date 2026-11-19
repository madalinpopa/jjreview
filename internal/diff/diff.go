package diff

type LineKind int

const (
	LineContext LineKind = iota
	LineAdded
	LineRemoved
)

type Line struct {
	Kind      LineKind
	Text      string
	OldNo     int
	NewNo     int
	NoNewline bool
}

type Hunk struct {
	OldStart int
	OldLines int
	NewStart int
	NewLines int
	Section  string
	Lines    []Line
}

type Status int

const (
	StatusModified Status = iota
	StatusAdded
	StatusDeleted
	StatusRenamed
	StatusCopied
)

type File struct {
	OldPath string
	NewPath string
	OldMode string
	NewMode string
	Status  Status
	Binary  bool
	Hunks   []Hunk
}

func (f File) Path() string {
	if f.Status == StatusDeleted {
		return f.OldPath
	}
	return f.NewPath
}
