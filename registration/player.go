//go:generate go tool stringer -type=ExperienceLevel

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
