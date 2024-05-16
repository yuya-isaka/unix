// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package v6unix

import (
	"unsafe"
)

// 参考: https://oraccha.hatenadiary.org/entry/20110920/1316512512

// UNIXではデバイスもファイルとして見せた方が汎用的で便利ではということで、スペシャルファイルが導入

// その際、デバイスはブロックデバイスとキャラクタデバイスの2種類に分けられた
// bdevとcdev

// bdev
// ディスクやテープなどの固定長のブロック単位で操作するデバイス
// 通常ファイルシステムにマウントして使用するデバイス
// 性能向上のために、バッファキャッシュを介してデータを読み書きする

// cdev
// TTYのように文字単位で操作するデバイス
// ↑元々の意味
// ↓
// 構造を持たないもの、/dev/nullなどの擬似デバイスなども含まれるようなった

// デバイスドライバの観点だと、cdevはbdevのように上位のバッファキャッシュとやり取りしなくていい

// Plan9では、それらの違いもユーザに見せない

// 実質的な違いは、バッファキャッシュを使うか否か
// ディスクをrawデバイスとして扱いたいなら cdev 使う

// ネットワークデバイスが入ってきてこのモデルは崩壊している
// デバイスの分類難しい

// UNIXカーネル
// ファイルシステムという皮を被ったI/O多重化装置

// デバイス毎に処理を多重化している部分=デバイススイッチ（bdevsw、cdevsw）
// デバイスの種類（つまりメジャー番号の数）だけ保持

// スイッチはオブジェクト指向的に設計

// bdevsw
//   	open、close、strategy

// cdevsw
//		open、close、read、write、sgtty（V7からioctlに改名）という関数ポインタ

// ↑デバイスドライバ毎に対応する関数が登録

type device interface {
	open(*Proc, uint8, int)
	read(*Proc, uint8, []byte, int) int
	write(*Proc, uint8, []byte, int) int
	close(*Proc, uint8)
	// 端末に対応したinodeを取得（ユーザからfdが渡され、ここではProc？）
	// そこからデバイスナンバーを取得
	sgtty(*Proc, uint8, *[3]uint16, *[3]uint16)
}

var devtab = []device{
	errdev{},
	nulldev{},
	memdev{},
	nulldev{}, // for /dev/swap
	ttydev{},
}

func (p *Proc) dev(major uint8) device {
	if int(major) >= len(devtab) || devtab[major] == nil {
		major = 0
	}
	return devtab[major]
}

type errdev struct{}

func (errdev) open(p *Proc, minor uint8, rw int) {
	p.Error = ENXIO
}

func (errdev) read(p *Proc, minor uint8, b []byte, off int) int {
	p.Error = ENXIO
	return 0
}

func (errdev) write(p *Proc, minor uint8, b []byte, off int) int {
	p.Error = ENXIO
	return 0
}

func (errdev) close(p *Proc, minor uint8) {
	p.Error = ENXIO
}

func (errdev) sgtty(p *Proc, minor uint8, in, out *[3]uint16) {
	p.Error = ENOTTY
}

type nulldev struct{}

func (nulldev) open(p *Proc, minor uint8, rw int) {
}

func (nulldev) read(p *Proc, minor uint8, b []byte, off int) int {
	return 0
}

func (nulldev) write(p *Proc, minor uint8, b []byte, off int) int {
	return len(b)
}

func (nulldev) close(p *Proc, minor uint8) {
}

func (nulldev) sgtty(p *Proc, minor uint8, in, out *[3]uint16) {
	p.Error = ENOTTY
}

const (
	// as listed in unix kernel
	memSwapDev = 0o001414
	memProcs   = 0o005206 // to 0o007322

	// arbitrary choices
	memTTY     = 0o002000 // to 0o002440
	memTTYSize = 16 * 2

	memText = 0o010000
)

type memdev struct{}

func (memdev) open(p *Proc, minor uint8, rw int) {
}

func (memdev) read(p *Proc, minor uint8, b []byte, off int) int {
	if off == memSwapDev && len(b) == 2 {
		// Asking for swap device minor, major.
		// As long as process table always has SLOAD, will never be used,
		// but must be able to open device.
		b[0] = 1
		b[1] = 3
		return 2
	}

	if off == memProcs {
		// Asking for procs table.
		var procs []procState
		for i, p1 := range p.Sys.Procs {
			p1.procState.flag |= _SLOAD

			// ps is going to use (p1.addr+p1.size-8)<<6 as the address
			// to read 512 bytes from.
			// Setting p1.size=8 zeros out the addend, leaving p1.addr.
			// We separate the process base addresses by 64 bytes to allow
			// packing many more into the "memory".
			p1.addr = uint16(memText/64 + i)
			p1.size = 8
			procs = append(procs, p1.procState)
		}
		pb := unsafe.Slice((*byte)(unsafe.Pointer(&procs[0])), len(procs)*int(unsafe.Sizeof(procState{})))
		clear(b)
		copy(b, pb)
		return len(pb)
	}

	if memText <= off && off&63 == 0 && off < memText+64*int(len(p.Sys.Procs)) && len(b) == 512 {
		p1 := p.Sys.Procs[(off-memText)/64]
		mem := p1.Mem[len(p.Mem)-512:]
		copy(b, mem)
		return len(b)
	}

	if memTTY <= off && off < memTTY+len(p.Sys.TTY)*memTTYSize && (off-memTTY)%memTTYSize == 0 && len(b) == memTTYSize {
		i := (off - memTTY) / memTTYSize
		tty := &p.Sys.TTY[i]
		tb := (*[unsafe.Sizeof(TDev{})]byte)(unsafe.Pointer(&tty.TDev))[:]
		clear(b)
		copy(b, tb)
		return len(tb)
	}

	return 0
}

func (memdev) write(p *Proc, minor uint8, b []byte, off int) int {
	p.Error = EPERM
	return 0
}

func (memdev) close(p *Proc, minor uint8) {
}

func (memdev) sgtty(p *Proc, minor uint8, in, out *[3]uint16) {
	p.Error = ENOTTY
}
