package media

type Kind string

const (
	Movie   Kind = "movie"
	Episode Kind = "episode"
)

type Status string

const (
	Unmatched   Status = "unmatched"
	Review      Status = "review"
	Ready       Status = "ready"
	Conflict    Status = "conflict"
	Unsupported Status = "unsupported"
	Error       Status = "error"
)

type Parsed struct {
	Kind         Kind
	Query        string
	Year         int
	Season       int
	Episode      int
	MultiEpisode bool
}

type Candidate struct {
	ID            int
	Kind          Kind
	Title         string
	OriginalTitle string
	Year          int
	SeriesYear    int
	Season        int
	Episode       int
	EpisodeTitle  string
}

type File struct {
	Path       string
	Parsed     Parsed
	Candidate  Candidate
	Candidates []Candidate
	Proposed   string
	Status     Status
	Message    string
}
