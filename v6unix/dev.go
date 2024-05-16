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

// おすすめ: https://www.tom-yam.or.jp/2238/ref/iosys.pdf

// V6でのシステムコール呼び出しからデバイススイッチへの流れ
// open(2)はnamei関数でパス名からiノード番号を探索オープンファイルテーブルをセットし、対応するインデックス（ファイル記述子）を返す。
// read(2)、write(2)では、オープンファイルテーブルからiノードを調べ、readi、writei関数を呼ぶ。
// 	readiはファイルがcdevならcdevsw.d_readを呼び、bdevならbreadやbreada経由でbdevsw.d_readを呼ぶ。

type device interface {
	open(*Proc, uint8, int)
	read(*Proc, uint8, []byte, int) int
	write(*Proc, uint8, []byte, int) int
	close(*Proc, uint8)
	// 端末に対応したinodeを取得（ユーザからfdが渡され、ここではProc？）
	// そこからデバイスナンバーを取得
	sgtty(*Proc, uint8, *[3]uint16, *[3]uint16)
}

// deviceインタフェースのスライス
// オブジェクトのリストを保持
var devtab = []device{
	errdev{},  // エラーデバイス
	nulldev{}, // ヌルデバイス
	memdev{},  // メモリデバイス
	nulldev{}, // for /dev/swap
	ttydev{},
}

func (p *Proc) dev(major uint8) device {
	if int(major) >= len(devtab) || devtab[major] == nil {
		major = 0
	}
	return devtab[major]
}

// エラーデバイス
// 全ての操作でエラーを返すデバイス
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

// 開いたりと閉じたりできる
// 書き込みはバイト数、読み込みは常に0
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
	// UNIXカーネルに記載されている通り
	memSwapDev = 0o001414
	memProcs   = 0o005206 // to 0o007322

	// arbitrary choices
	// 任意の選択
	memTTY     = 0o002000 // to 0o002440  0o002440まで
	memTTYSize = 16 * 2

	// テキストセグメントの開始位置？
	memText = 0o010000
)

// 特定のメモリ領域を模倣するデバイス
type memdev struct{}

// 特定のプロセス(p)、マイナー番号(minor)、および読み書きモード(rw)を引数に取りますが、現在は何も実行しない
func (memdev) open(p *Proc, minor uint8, rw int) {
}

// オフセットに基づいて動作が異なる
// 読み出したデータの長さを返す
func (memdev) read(p *Proc, minor uint8, b []byte, off int) int {
	// offがmemSwapDevと等しく、bの長さが2の場合、スワップデバイスのマイナーとメジャーを要求
	if off == memSwapDev && len(b) == 2 {
		// スワップデバイスのマイナー、メジャーを要求しています。
		// プロセステーブルが常にSLOADを持つ限り、使用されることはありませんが、
		// デバイスを開くことができなければなりません。
		b[0] = 1
		b[1] = 3
		return 2
	}

	// offがmemProcsと等しい場合、プロセステーブルを要求
	// このコードは、プロセステーブルの各エントリに対して特定の操作を行い、その結果をbにコピー
	if off == memProcs {
		// プロセステーブルを要求しています。
		var procs []procState
		for i, p1 := range p.Sys.Procs {
			p1.procState.flag |= _SLOAD

			// psは(p1.addr+p1.size-8)<<6をアドレスとして
			// 512バイトを読み出すつもりです。
			// p1.size=8を設定すると、加算部分がゼロになり、p1.addrが残ります。
			// プロセスの基本アドレスを64バイトごとに分けることで、
			// "メモリ"に多くのプロセスを詰め込むことができます。
			p1.addr = uint16(memText/64 + i)
			p1.size = 8
			procs = append(procs, p1.procState)
		}
		pb := unsafe.Slice((*byte)(unsafe.Pointer(&procs[0])), len(procs)*int(unsafe.Sizeof(procState{})))
		clear(b)
		copy(b, pb)
		return len(pb)
	}

	// offがmemTextとmemTextにプロセスの数を64倍して足した値の間で、
	// offが64の倍数で、
	// bの長さが512の場合、
	// 特定のプロセスのメモリを読み出し
	if memText <= off && off < memText+64*int(len(p.Sys.Procs)) && off&63 == 0 && len(b) == 512 {
		// offとmemTextはおそらくメモリオフセットとテキストセグメントの開始位置
		// これらの差を64で割ることで、特定のプロセスを指すインデックスを計算
		p1 := p.Sys.Procs[(off-memText)/64]
		// 取得したプロセスp1のメモリ領域から最後の512バイトを取得
		mem := p1.Mem[len(p.Mem)-512:]
		copy(b, mem)
		return len(b)
	}

	// offがmemTTYとmemTTYにTTYの数をmemTTYSize倍した値の間で、
	// offからmemTTYを引いた値がmemTTYSizeの倍数で、
	// bの長さがmemTTYSizeの場合
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

// EPERM（許可されていない操作）エラーを設定し、0を返すだけ
func (memdev) write(p *Proc, minor uint8, b []byte, off int) int {
	p.Error = EPERM
	return 0
}

func (memdev) close(p *Proc, minor uint8) {
}

// ENOTTY（不適切な ioctl（入出力制御））エラーを設定するだけ
func (memdev) sgtty(p *Proc, minor uint8, in, out *[3]uint16) {
	p.Error = ENOTTY
}
