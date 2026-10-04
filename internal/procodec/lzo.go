package procodec

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// DecodeLZO1X decompresses a bare LZO1X-1 stream into at most maxOut bytes.
//
// Procreate's legacy ".chunk" tiles are raw LZO1X-1 with no container header at
// all; the giveaway is the 11 00 00 end-of-stream marker every one of them ends
// with. There is no declared output size in the stream, so the caller passes
// the expected tile size as the bound.
//
// The control flow mirrors minilzo's lzo1x_decompress one branch at a time,
// expressed as an explicit state machine because Go forbids the jumps into
// blocks that the C original uses.
func DecodeLZO1X(src []byte, maxOut int) ([]byte, error) {
	const (
		stTop = iota
		stFirstLiteralRun
		stMatch
		stMatchDone
		stMatchNext
	)

	out := make([]byte, 0, min(maxOut, 1<<20))
	ip, t, state := 0, 0, stTop

	need := func(n int) error {
		if ip+n > len(src) {
			return errors.New("procodec: truncated LZO stream")
		}
		return nil
	}
	copyLit := func(n int) error {
		if err := need(n); err != nil {
			return err
		}
		if len(out)+n > maxOut {
			return errOverrun(len(out), n, maxOut)
		}
		out = append(out, src[ip:ip+n]...)
		ip += n
		return nil
	}
	copyMatch := func(dist, n int) error {
		if dist <= 0 || dist > len(out) {
			return fmt.Errorf("procodec: bad LZO distance %d (history %d)", dist, len(out))
		}
		if len(out)+n > maxOut {
			return errOverrun(len(out), n, maxOut)
		}
		s := len(out) - dist
		for i := 0; i < n; i++ {
			out = append(out, out[s+i])
		}
		return nil
	}
	// extend reads the 255-chained long length shared by literal runs, M3 and M4.
	extend := func(base int) (int, error) {
		n := 0
		for ip < len(src) && src[ip] == 0 {
			n += 255
			ip++
		}
		if err := need(1); err != nil {
			return 0, err
		}
		n += base + int(src[ip])
		ip++
		return n, nil
	}

	if err := need(1); err != nil {
		return nil, err
	}
	if src[ip] > 17 {
		t = int(src[ip]) - 17
		ip++
		if t < 4 {
			state = stMatchNext
		} else {
			if err := copyLit(t); err != nil {
				return nil, err
			}
			state = stFirstLiteralRun
		}
	}

	for {
		switch state {
		case stTop:
			if err := need(1); err != nil {
				return nil, err
			}
			t = int(src[ip])
			ip++
			if t >= 16 {
				state = stMatch
				continue
			}
			if t == 0 {
				n, err := extend(15)
				if err != nil {
					return nil, err
				}
				t = n
			}
			if err := copyLit(t + 3); err != nil {
				return nil, err
			}
			state = stFirstLiteralRun

		case stFirstLiteralRun:
			if err := need(1); err != nil {
				return nil, err
			}
			t = int(src[ip])
			ip++
			if t >= 16 {
				state = stMatch
				continue
			}
			if err := need(1); err != nil {
				return nil, err
			}
			// The literal run that opens a stream is followed by a match whose
			// distance is biased past M2's 2048-byte window.
			dist := 1 + 2048 + (t >> 2) + int(src[ip])<<2
			ip++
			if err := copyMatch(dist, 3); err != nil {
				return nil, err
			}
			state = stMatchDone

		case stMatch:
			switch {
			case t >= 64: // M2
				if err := need(1); err != nil {
					return nil, err
				}
				dist := 1 + ((t >> 2) & 7) + int(src[ip])<<3
				ip++
				if err := copyMatch(dist, (t>>5)+1); err != nil {
					return nil, err
				}
			case t >= 32: // M3
				t &= 31
				if t == 0 {
					n, err := extend(31)
					if err != nil {
						return nil, err
					}
					t = n
				}
				if err := need(2); err != nil {
					return nil, err
				}
				dist := 1 + int(binary.LittleEndian.Uint16(src[ip:])>>2)
				ip += 2
				if err := copyMatch(dist, t+2); err != nil {
					return nil, err
				}
			case t >= 16: // M4, which also encodes end of stream
				back := (t & 8) << 11
				t &= 7
				if t == 0 {
					n, err := extend(7)
					if err != nil {
						return nil, err
					}
					t = n
				}
				if err := need(2); err != nil {
					return nil, err
				}
				back += int(binary.LittleEndian.Uint16(src[ip:]) >> 2)
				ip += 2
				if back == 0 {
					return out, nil
				}
				if err := copyMatch(back+0x4000, t+2); err != nil {
					return nil, err
				}
			default: // M1
				if err := need(1); err != nil {
					return nil, err
				}
				dist := 1 + (t >> 2) + int(src[ip])<<2
				ip++
				if err := copyMatch(dist, 2); err != nil {
					return nil, err
				}
			}
			state = stMatchDone

		case stMatchDone:
			if ip < 2 {
				return nil, errors.New("procodec: malformed LZO stream")
			}
			// The low two bits of the last distance byte pair carry the length
			// of the literal run that follows the match.
			t = int(src[ip-2]) & 3
			if t == 0 {
				state = stTop
				continue
			}
			state = stMatchNext

		case stMatchNext:
			if err := copyLit(t); err != nil {
				return nil, err
			}
			if err := need(1); err != nil {
				return nil, err
			}
			t = int(src[ip])
			ip++
			state = stMatch
		}
	}
}
