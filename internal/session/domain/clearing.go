package domain

// Clearing is whether a pane's before commands are hidden once they ran,
// as the project, a window or a pane sets it: the lowest level that sets
// it wins, and a pane clears only when one sets it on.
type Clearing int

const (
	// ClearingUnset leaves it to the level above.
	ClearingUnset Clearing = iota
	// ClearingOn types clear after the before commands.
	ClearingOn
	// ClearingOff leaves the before commands shown.
	ClearingOff
)

// Over is c, or above when c is unset.
func (c Clearing) Over(above Clearing) Clearing {
	if c == ClearingUnset {
		return above
	}
	return c
}

// IsSet tells whether c is set, on or off.
func (c Clearing) IsSet() bool {
	return c != ClearingUnset
}
