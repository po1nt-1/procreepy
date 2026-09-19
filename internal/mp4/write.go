package mp4

import "encoding/binary"

// Low-level big-endian encoders.
func be16(v uint16) []byte {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, v)
	return b
}

func be32(v uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, v)
	return b
}

func be64(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)
	return b
}

// NewBox builds a plain box: size(4) + type(4) + payload.
func NewBox(fourcc string, payload []byte) []byte {
	out := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(out, uint32(8+len(payload)))
	copy(out[4:8], fourcc)
	copy(out[8:], payload)
	return out
}

// FullBox builds a full box: size(4) + type(4) + version(1) + flags(3) +
// payload, where payload is the content that follows version and flags.
func FullBox(fourcc string, version byte, flags uint32, payload []byte) []byte {
	head := []byte{version, byte(flags >> 16), byte(flags >> 8), byte(flags)}
	body := make([]byte, 0, len(head)+len(payload))
	body = append(body, head...)
	body = append(body, payload...)
	return NewBox(fourcc, body)
}

// Container builds a container box whose body is the concatenation of child
// boxes.
func Container(fourcc string, children ...[]byte) []byte {
	total := 0
	for _, c := range children {
		total += len(c)
	}
	out := make([]byte, 8+total)
	binary.BigEndian.PutUint32(out, uint32(8+total))
	copy(out[4:8], fourcc)
	p := 8
	for _, c := range children {
		copy(out[p:], c)
		p += len(c)
	}
	return out
}

var identityMatrix = func() []byte {
	m := make([]byte, 36)
	binary.BigEndian.PutUint32(m[0:4], 0x00010000)
	binary.BigEndian.PutUint32(m[16:20], 0x00010000)
	binary.BigEndian.PutUint32(m[32:36], 0x40000000)
	return m
}()

// encodeMatrix renders a 3x3 16.16-fixed-point matrix as 36 bytes.
func encodeMatrix(m [9]int64) []byte {
	out := make([]byte, 36)
	for i, v := range m {
		binary.BigEndian.PutUint32(out[i*4:], uint32(v))
	}
	return out
}

// identityMatrixVals is the canonical 3x3 identity in 16.16 fixed point.
var identityMatrixVals = [9]int64{
	0x00010000, 0, 0,
	0, 0x00010000, 0,
	0, 0, 0x40000000,
}

// EncFTYP returns a minimal ftyp box (major brand "isom").
func EncFTYP() []byte {
	payload := append([]byte("isom"), be32(0x200)...)
	payload = append(payload, []byte("isomiso2")...)
	return NewBox("ftyp", payload)
}

// EncMVHD builds the movie header.
func EncMVHD(timescale uint32, duration uint64) []byte {
	if duration >= 1<<32 {
		payload := make([]byte, 8)                    // creation
		payload = append(payload, make([]byte, 8)...) // modification
		payload = append(payload, be32(timescale)...)
		payload = append(payload, be64(duration)...)
		payload = append(payload, be32(0x00010000)...) // rate 1.0
		payload = append(payload, be16(0x0100)...)     // volume 1.0
		payload = append(payload, make([]byte, 10)...) // reserved(2)+reserved(8)
		payload = append(payload, identityMatrix...)
		payload = append(payload, make([]byte, 12)...) // pre_defined[3]
		payload = append(payload, be32(0xffffffff)...) // next track ID
		return FullBox("mvhd", 1, 0, payload)
	}
	payload := make([]byte, 4)                    // creation
	payload = append(payload, make([]byte, 4)...) // modification
	payload = append(payload, be32(timescale)...)
	payload = append(payload, be32(uint32(duration))...)
	payload = append(payload, be32(0x00010000)...) // rate 1.0
	payload = append(payload, be16(0x0100)...)     // volume 1.0
	payload = append(payload, make([]byte, 10)...) // reserved(2)+reserved(8)
	payload = append(payload, identityMatrix...)
	payload = append(payload, make([]byte, 12)...) // pre_defined[3]
	payload = append(payload, be32(0xffffffff)...) // next track ID
	return FullBox("mvhd", 0, 0, payload)
}

// EncTKHD builds a track header. duration is in the movie timescale. For
// audio tracks pass w=h=0; the matrix carries rotation/orientation.
func EncTKHD(trackID uint32, duration uint64, w, h uint16, volume uint16, matrix [9]int64) []byte {
	const flags = uint32(3) // enabled + in movie
	mat := encodeMatrix(matrix)
	dur32 := duration < 1<<32
	version := byte(0)
	if !dur32 {
		version = 1
	}
	var payload []byte
	switch {
	case !dur32:
		payload = make([]byte, 8)                     // creation
		payload = append(payload, make([]byte, 8)...) // modification
		payload = append(payload, be32(trackID)...)
		payload = append(payload, make([]byte, 4)...) // reserved
		payload = append(payload, be64(duration)...)
		payload = append(payload, make([]byte, 8)...) // reserved
	default:
		payload = make([]byte, 4)                     // creation
		payload = append(payload, make([]byte, 4)...) // modification
		payload = append(payload, be32(trackID)...)
		payload = append(payload, make([]byte, 4)...) // reserved
		payload = append(payload, be32(uint32(duration))...)
		payload = append(payload, make([]byte, 8)...) // reserved
	}
	payload = append(payload, be16(0)...)      // layer
	payload = append(payload, be16(0)...)      // alternate group
	payload = append(payload, be16(volume)...) // volume
	payload = append(payload, be16(0)...)      // reserved
	payload = append(payload, mat...)
	payload = append(payload, be32(uint32(w)<<16)...)
	payload = append(payload, be32(uint32(h)<<16)...)
	return FullBox("tkhd", version, flags, payload)
}

// EncMDHD builds the media header.
func EncMDHD(timescale uint32, duration uint64, language uint32) []byte {
	if duration >= 1<<32 {
		payload := make([]byte, 8)                    // creation
		payload = append(payload, make([]byte, 8)...) // modification
		payload = append(payload, be32(timescale)...)
		payload = append(payload, be64(duration)...)
		payload = append(payload, be32(language)...)
		payload = append(payload, be16(0)...) // quality
		return FullBox("mdhd", 1, 0, payload)
	}
	payload := make([]byte, 4)                    // creation
	payload = append(payload, make([]byte, 4)...) // modification
	payload = append(payload, be32(timescale)...)
	payload = append(payload, be32(uint32(duration))...)
	payload = append(payload, be32(language)...)
	payload = append(payload, be16(0)...) // quality
	return FullBox("mdhd", 0, 0, payload)
}

// EncHDLR builds a handler reference box.
func EncHDLR(handler, name string) []byte {
	payload := make([]byte, 4) // predefined
	payload = append(payload, []byte(pad4(handler))...)
	payload = append(payload, make([]byte, 12)...) // predefined
	payload = append(payload, []byte(name)...)
	payload = append(payload, 0) // null terminate
	return FullBox("hdlr", 0, 0, payload)
}

func pad4(s string) string {
	if len(s) > 4 {
		return s[:4]
	}
	out := []byte(s)
	for len(out) < 4 {
		out = append(out, 0)
	}
	return string(out)
}

// EncVMHD builds the video media header.
func EncVMHD() []byte {
	payload := make([]byte, 2)                    // graphicsmode
	payload = append(payload, make([]byte, 6)...) // opcolor
	return FullBox("vmhd", 0, 1, payload)
}

// EncSMHD builds the sound media header.
func EncSMHD() []byte {
	payload := make([]byte, 2)                    // balance
	payload = append(payload, make([]byte, 2)...) // reserved
	return FullBox("smhd", 0, 0, payload)
}

// EncDINF builds a data information box with a single self-contained url.
func EncDINF() []byte {
	url := FullBox("url ", 0, 1, nil) // self-contained
	drefPayload := be32(1)            // entry count
	drefPayload = append(drefPayload, url...)
	dref := FullBox("dref", 0, 0, drefPayload)
	return NewBox("dinf", dref)
}

// EncSTTS builds a decode-time-to-sample table.
func EncSTTS(entries []SttsEntry) []byte {
	payload := be32(uint32(len(entries)))
	for _, e := range entries {
		payload = append(payload, be32(e.Count)...)
		payload = append(payload, be32(e.Delta)...)
	}
	return FullBox("stts", 0, 0, payload)
}

// EncSTSC builds a sample-to-chunk table.
func EncSTSC(entries []StscEntry) []byte {
	payload := be32(uint32(len(entries)))
	for _, e := range entries {
		payload = append(payload, be32(e.FirstChunk)...)
		payload = append(payload, be32(e.SamplesPerChunk)...)
		payload = append(payload, be32(e.Description)...)
	}
	return FullBox("stsc", 0, 0, payload)
}

// EncSTSZ builds a sample-size table, using a uniform size when possible.
// Layout after version+flags: sample_size(4) + sample_count(4) [+ per-sample].
func EncSTSZ(sizes []uint32) []byte {
	if len(sizes) > 0 {
		uniform := sizes[0]
		allSame := true
		for _, s := range sizes[1:] {
			if s != uniform {
				allSame = false
				break
			}
		}
		if allSame {
			payload := be32(uniform)
			payload = append(payload, be32(uint32(len(sizes)))...)
			return FullBox("stsz", 0, 0, payload)
		}
	}
	payload := be32(0)                                     // sample_size = 0, per-sample table follows
	payload = append(payload, be32(uint32(len(sizes)))...) // sample_count
	for _, s := range sizes {
		payload = append(payload, be32(s)...)
	}
	return FullBox("stsz", 0, 0, payload)
}

// EncSTCO builds a chunk-offset table. use64 forces co64; otherwise stco is
// used (offsets must all be < 4 GiB).
func EncSTCO(offsets []uint64, use64 bool) []byte {
	payload := be32(uint32(len(offsets)))
	if use64 {
		for _, o := range offsets {
			payload = append(payload, be64(o)...)
		}
		return FullBox("co64", 0, 0, payload)
	}
	for _, o := range offsets {
		payload = append(payload, be32(uint32(o))...)
	}
	return FullBox("stco", 0, 0, payload)
}

// EncSTSS builds a sync-sample table.
func EncSTSS(samples []uint32) []byte {
	payload := be32(uint32(len(samples)))
	for _, s := range samples {
		payload = append(payload, be32(s)...)
	}
	return FullBox("stss", 0, 0, payload)
}

// EncCTTS builds a composition-offset table. signed selects version 1.
func EncCTTS(entries []CttsEntry, signed bool) []byte {
	version := byte(0)
	if signed {
		version = 1
	}
	payload := be32(uint32(len(entries)))
	for _, e := range entries {
		payload = append(payload, be32(e.Count)...)
		payload = append(payload, be32(uint32(int32(e.Offset)))...)
	}
	return FullBox("ctts", version, 0, payload)
}
