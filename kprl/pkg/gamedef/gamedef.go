// Package gamedef defines game types, encryption keys, and target engine
// configurations for RealLive-based visual novels.
// Transposed from OCaml's gameTypes.ml + game.ml.
package gamedef

import "fmt"

// TargetEngine represents the game engine type.
type TargetEngine int

const (
	EngineRealLive TargetEngine = iota
	EngineKinetic
	EngineAvg2000
	EngineSiglus
)

func (t TargetEngine) String() string {
	switch t {
	case EngineRealLive:
		return "RealLive"
	case EngineKinetic:
		return "Kinetic"
	case EngineAvg2000:
		return "Avg2000"
	case EngineSiglus:
		return "Siglus"
	default:
		return "Unknown"
	}
}

// Version represents a 4-part engine version number.
type Version struct {
	Major, Minor, Patch, Build int
}

func (v Version) String() string {
	if v.Build != 0 {
		return fmt.Sprintf("%d.%d.%d.%d", v.Major, v.Minor, v.Patch, v.Build)
	}
	if v.Patch != 0 {
		return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	}
	return fmt.Sprintf("%d.%d", v.Major, v.Minor)
}

// Target represents an engine target specification.
type Target struct {
	Engine  TargetEngine
	Version *Version // nil = any version
	Compat  int      // -1 = any
}

// XORSubkey represents one XOR encryption sub-key with its position and range.
// Transposed from OCaml's subkey_t record type.
type XORSubkey struct {
	Offset int
	Length int
	Data   [16]byte
}

// GameDef holds the complete definition for a game.
// Transposed from OCaml's game_t record type.
type GameDef struct {
	ID       string
	Title    string
	By       string      // Publisher
	Seens    int         // Number of SEEN files (-1 = unknown)
	Inherits []string    // Parent game IDs
	Target   *Target     // Engine target (nil = default)
	Key      []XORSubkey // XOR encryption keys (nil = none)
	Dir      string      // Game directory
}

// EmptyGame returns a default empty game definition.
func EmptyGame() GameDef {
	return GameDef{Seens: -1}
}

// NewGame creates a new game definition.
func NewGame(id, title, by string, seens int, inherits []string, target *Target, key []XORSubkey) GameDef {
	return GameDef{
		ID:       id,
		Title:    title,
		By:       by,
		Seens:    seens,
		Inherits: inherits,
		Target:   target,
		Key:      key,
	}
}

// GetXORKeys returns the XOR keys for the game (empty slice if none).
func (g *GameDef) GetXORKeys() []XORSubkey {
	if g.Key == nil {
		return nil
	}
	return g.Key
}

// KeyString returns a hex string representation of a subkey's data.
func (sk *XORSubkey) KeyString() string {
	s := ""
	for i := 0; i < 16; i++ {
		if i > 0 {
			s += " "
		}
		s += fmt.Sprintf("%02x", sk.Data[i])
	}
	return s
}
