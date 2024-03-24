package sillyname

import (
	"fmt"
	"math/rand"
)

// Simple nink name generator
// Returns a silly name (trhee parts randomly combined from a list)

var (
	// Names is a list of silly names
	Names = Load()
)

// Load names from library
func Load() []string {
	// TODO: load from accets
	return []string{
		"Alpha",
		"Atomic",
		"Blue",
		"Byte",
		"Cipher",
		"Code",
		"Cool",
		"Count",
		"Data",
		"Echo",
		"Elite",
		"Fast",
		"Firewall",
		"Green",
		"Hot",
		"Ice",
		"Joker",
		"Kernel",
		"Killer",
		"Lucid",
		"Master",
		"Metal",
		"Ninja",
		"Omega",
		"Orange",
		"Phantom",
		"Protocol",
		"Quantum",
		"Quick",
		"Red",
		"Rogue",
		"Shadow",
		"Stealth",
		"Virus",
		"Vortex",
		"Warden",
		"White",
		"Xenon",
		"Yellow",
		"Zero",
		"Zombie",
	}
}

// Generate a silly name
func Generate() string {
	ln := len(Names)
	n1 := Names[rand.Intn(ln)]
	n2 := Names[rand.Intn(ln)]
	n3 := Names[rand.Intn(ln)]
	return fmt.Sprintf("%s.%s.%s", n1, n2, n3)
}
