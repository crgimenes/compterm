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
		"Dark",
		"Echo",
		"Elite",
		"Fast",
		"Firewall",
		"Green",
		"Ghost",
		"Hot",
		"Hyper",
		"Ice",
		"Iron",
		"Joker",
		"Jumbo",
		"Kernel",
		"Killer",
		"Lucid",
		"Light",
		"Magic",
		"Master",
		"Metal",
		"Ninja",
		"Omega",
		"Orange",
		"Phantom",
		"Protocol",
		"Purple",
		"Quantum",
		"Quick",
		"Red",
		"Rapid",
		"Replica",
		"Rogue",
		"Shadow",
		"Stealth",
		"Signal",
		"Uncheked",
		"Ultra",
		"True",
		"Turbo",
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
