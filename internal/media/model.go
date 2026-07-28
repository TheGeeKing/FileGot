package media

import "github.com/TheGeeKing/FileGot/internal/mediainfo"

type Kind string

const (
	Movie   Kind = "movie"
	Episode Kind = "episode"
)

type Status string

const (
	Expected    Status = "expected"
	Unmatched   Status = "unmatched"
	Review      Status = "review"
	Ready       Status = "ready"
	Conflict    Status = "conflict"
	Unsupported Status = "unsupported"
	Metadata    Status = "metadata"
	SameName    Status = "same name"
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
	ID                   int
	Kind                 Kind
	Title                string
	OriginalTitle        string
	PosterPath           string
	Overview             string
	Year                 int
	SeriesYear           int
	Season               int
	Episode              int
	EpisodeTMDBID        int
	EpisodeTitle         string
	OriginalEpisodeTitle string
	ReleaseDate          string
	AirDate              string
	OriginalLanguage     string
	Genre                string
	LawRating            string
	Directors            []string
	Writers              []string
	Actors               []string
}

type File struct {
	Path            string
	Imported        bool
	Parsed          Parsed
	Candidate       Candidate
	Candidates      []Candidate
	Proposed        string
	Status          Status
	Message         string
	Technical       mediainfo.Metadata
	MetadataPending bool
}

func (file File) IsExpectedEpisode() bool {
	return file.Imported && file.Path == "" && file.Status == Expected
}

func (file File) IsEpisodePairing() bool {
	return file.Imported && file.Path != ""
}
