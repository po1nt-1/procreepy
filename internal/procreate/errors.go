package procreate

import "errors"

// Sentinel errors. The CLI layer maps them onto exit codes:
// ErrInput -> 3, ErrNoSegments -> 4, ErrBadSegment -> 5.

var (
	// ErrInput: the input file is missing, empty, or not a valid ZIP.
	ErrInput = errors.New("input")

	// ErrNoSegments: the archive has no video/segments.
	ErrNoSegments = errors.New("no video segments")

	// ErrBadSegment: ambiguous segment numbering or a corrupted member.
	ErrBadSegment = errors.New("bad segment")
)

// Typed errors carry the user-facing message; they unwrap to their sentinel
// so errors.Is keeps working while Error() shows no sentinel prefix.

// InputError reports a missing or unusable input (exit 3).
type InputError struct{ Msg string }

func (e *InputError) Error() string { return e.Msg }
func (e *InputError) Unwrap() error { return ErrInput }

// NoSegmentsError reports an archive without timelapse segments (exit 4).
type NoSegmentsError struct{ Msg string }

func (e *NoSegmentsError) Error() string { return e.Msg }
func (e *NoSegmentsError) Unwrap() error { return ErrNoSegments }

// BadSegmentError reports ambiguous numbering or a corrupted segment (exit 5).
type BadSegmentError struct{ Msg string }

func (e *BadSegmentError) Error() string { return e.Msg }
func (e *BadSegmentError) Unwrap() error { return ErrBadSegment }
