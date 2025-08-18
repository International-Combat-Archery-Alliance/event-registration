package registration

type PlayerInfo struct {
	FirstName string
	LastName  string
}

type ExperienceLevel int

const (
	NOVICE ExperienceLevel = iota
	INTERMEDIATE
	ADVANCED
)
