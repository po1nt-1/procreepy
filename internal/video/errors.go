package video

// Error kinds. The CLI layer maps them onto exit codes:
// UsageError -> 2, IncompatibleError -> 7, WriteError -> 9.
// (procreate sentinels map to 3/4/5; anything else maps to 1.)

// UsageError is a bad command line (wrong arguments, terminal targets).
type UsageError struct{ Msg string }

func (e *UsageError) Error() string { return e.Msg }

// IncompatibleError: the segments cannot be joined with stream copy.
type IncompatibleError struct{ Msg string }

func (e *IncompatibleError) Error() string { return e.Msg }

// WriteError: the output (or the temp spool) could not be written.
type WriteError struct{ Msg string }

func (e *WriteError) Error() string { return e.Msg }
