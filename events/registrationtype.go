//go:generate go tool stringer -type=RegistrationType

package events

type RegistrationType int

const (
	BY_INDIVIDUAL RegistrationType = iota
	BY_TEAM
)
