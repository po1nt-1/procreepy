package video

import (
	"os"
	"syscall"
	"time"
)

// createTimeOf reads the file's creation time, which Windows records and
// exposes through the stat data. A zero result means it could not be read.
func createTimeOf(fi os.FileInfo) time.Time {
	d, ok := fi.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return time.Time{}
	}
	return time.Unix(0, d.CreationTime.Nanoseconds())
}

// setCreateTime stamps the file's creation time, which Windows keeps as a real
// attribute and Explorer shows in its "Date created" column.
//
// Without this, a slimmed project published through a scratch file reports the
// moment of conversion as its creation date, while the artwork it stands for
// may be years old. The rename that publishes the file keeps the attribute: a
// move within a volume carries the record, so stamping the scratch file before
// the rename is enough.
//
// FILE_WRITE_ATTRIBUTES is the narrowest right that allows SetFileTime, and the
// share flags let other readers keep the file open meanwhile.
func setCreateTime(path string, t time.Time) error {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	h, err := syscall.CreateFile(p, syscall.FILE_WRITE_ATTRIBUTES,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return err
	}
	defer syscall.CloseHandle(h)
	ft := syscall.NsecToFiletime(t.UnixNano())
	return syscall.SetFileTime(h, &ft, nil, nil)
}
