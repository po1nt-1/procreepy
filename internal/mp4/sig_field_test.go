package mp4

import "testing"

// TestTrackSigPerField pins that the stream-copy compatibility signature
// reacts to every field it declares.
func TestTrackSigPerField(t *testing.T) {
	base := TrackState{
		Handler: "vide", CodecName: "h264", FourCC: "avc1",
		Width: 320, Height: 240, PixFmt: "yuv420p",
		Timescale: 30, AudioRate: 44100, AudioChans: 2,
		ConfigBox: "avcC", ConfigData: []byte{1, 2, 3},
		Matrix: identityMatrixVals,
	}
	if !base.sig().same(base.sig()) {
		t.Fatal("identical track states do not match")
	}
	// pixfmt is deliberately absent: it is display-only (describe()) and
	// derived from the codec configuration, which same() compares.
	muts := map[string]func(*TrackState){
		"handler":     func(x *TrackState) { x.Handler = "soun" }, // also flips kind
		"fourcc":      func(x *TrackState) { x.FourCC = "avc3" },
		"width":       func(x *TrackState) { x.Width = 322 },
		"height":      func(x *TrackState) { x.Height = 242 },
		"rate":        func(x *TrackState) { x.AudioRate = 48000 },
		"chans":       func(x *TrackState) { x.AudioChans = 1 },
		"ts":          func(x *TrackState) { x.Timescale = 60 },
		"config":      func(x *TrackState) { x.ConfigBox = "hvcC" },
		"confDat":     func(x *TrackState) { x.ConfigData = []byte{1, 2, 4} },
		"confDat-nil": func(x *TrackState) { x.ConfigData = nil },
		"matrix":      func(x *TrackState) { x.Matrix[0] = 0x18000 },
	}
	for name, mut := range muts {
		a, b := base, base
		mut(&b)
		if a.sig().same(b.sig()) {
			t.Errorf("%s: same() = true, want false", name)
		}
	}
}

// TestTrackSigStssIgnored pins the product decision: the stss table is
// per-segment keyframe placement, not stream-copy compatibility, so
// sync-table presence and content must not affect the signature.
func TestTrackSigStssIgnored(t *testing.T) {
	base := TrackState{
		Handler: "vide", CodecName: "h264", FourCC: "avc1",
		Width: 320, Height: 240, PixFmt: "yuv420p",
		Timescale: 30, AudioRate: 44100, AudioChans: 2,
		ConfigBox: "avcC", ConfigData: []byte{1, 2, 3},
		Matrix: identityMatrixVals,
	}
	withSync := base
	withSync.HasSyncTable = true
	withSync.SyncSamples = []uint32{1, 2}
	if !base.sig().same(withSync.sig()) {
		t.Error("stss presence must not change the compatibility signature")
	}
	otherSync := base
	otherSync.HasSyncTable = true
	otherSync.SyncSamples = []uint32{2}
	if !withSync.sig().same(otherSync.sig()) {
		t.Error("different sync sample sets must not change the signature")
	}
}
