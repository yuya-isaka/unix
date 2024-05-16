// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package v6unix

import "fmt"

// なぜint8なのか
type Errno int8

// UNIXのエラーコード
const (
	EPERM Errno = 1 + iota
	ENOENT
	ESRCH
	EINTR
	EIO
	ENXIO
	E2BIG
	ENOEXEC
	EBADF
	ECHILD
	EAGAIN
	ENOMEM
	EACCES
	ENOTBLK
	EBUSY
	EEXIST
	EXDEV
	ENODEV
	ENOTDIR
	EISDIR
	EINVAL
	ENFILE
	EMFILE
	ENOTTY
	ETXTBSY
	EFBIG
	ENOSPC
	ESPIPE
	EROFS
	EMLINK
	EPIPE
	EFAULT Errno = 106
)

// エラーコードを受け取る
func (e Errno) Error() string {
	// 最後の106
	if e == EFAULT {
		return "EFAULT"
	}

	// エラーコードから文字列を取得
	// 長さチェックしてからスライスにアクセスしようね〜
	if 0 <= e && int(e) < len(enames) && enames[e] != "" {
		return enames[e]
	}
	return fmt.Sprintf("Errno(%d)", int(e))
}

// エラーコードと文字列の対応
var enames = []string{
	"",
	"EPERM",
	"ENOENT",
	"ESRCH",
	"EINTR",
	"EIO",
	"ENXIO",
	"E2BIG",
	"ENOEXEC",
	"EBADF",
	"ECHILD",
	"EAGAIN",
	"ENOMEM",
	"EACCES",
	"ENOTBLK",
	"EBUSY",
	"EEXIST",
	"EXDEV",
	"ENODEV",
	"ENOTDIR",
	"EISDIR",
	"EINVAL",
	"ENFILE",
	"EMFILE",
	"ENOTTY",
	"ETXTBSY",
	"EFBIG",
	"ENOSPC",
	"ESPIPE",
	"EROFS",
	"EMLINK",
	"EPIPE",
}
