package events

type Location struct {
	Name       string
	LocAddress Address
}

type Address struct {
	Street     string
	City       string
	State      string
	PostalCode string
	Country    string
}
