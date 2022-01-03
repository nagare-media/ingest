/*
Copyright 2022 The nagare media authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package mp4

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"

	mp4ff "github.com/edgeware/mp4ff/mp4"
)

var (
	ErrNotACMAFHeader     = errors.New("not a CMAF header")
	ErrNotACMAFChunk      = errors.New("not a CMAF chunk")
	ErrNotACMAFFragment   = errors.New("not a CMAF fragment")
	ErrTrackUninitialized = errors.New("track is uninitialized")
)

const (
	boxSizeLen   = 4
	boxNameLen   = 4
	boxHeaderLen = boxSizeLen + boxNameLen
	largeSizeLen = 8

	hdrDecBufSize = 8 // = max(boxHeaderLen, largeSizeLen)
)

// TODO: benchmark this as buffer is really small
var hdrDecBufPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, hdrDecBufSize)
	},
}

// adopted from mp4ff box.go
func DecodeHeader(hdr *mp4ff.BoxHeader, r io.Reader) error {
	buf := hdrDecBufPool.Get().([]byte)
	defer hdrDecBufPool.Put(buf) // nolint
	buf = buf[0:boxHeaderLen]
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return err
	}
	size := uint64(binary.BigEndian.Uint32(buf[0:4]))
	headerLen := boxHeaderLen
	if size == 1 {
		buf := hdrDecBufPool.Get().([]byte)
		defer hdrDecBufPool.Put(buf) // nolint
		buf = buf[0:largeSizeLen]
		_, err = io.ReadFull(r, buf)
		if err != nil {
			return err
		}
		size = binary.BigEndian.Uint64(buf)
		headerLen += largeSizeLen
	} else if size == 0 {
		return fmt.Errorf("Size 0, meaning to end of file, not supported")
	}
	hdr.Name = string(buf[4:8])
	hdr.Size = size
	hdr.Hdrlen = headerLen
	return nil
}

func EncodeHeader(hdr *mp4ff.BoxHeader, buf []byte) (n int, err error) {
	if len(buf) < boxHeaderLen+largeSizeLen {
		return 0, io.ErrShortBuffer
	}

	largeSize := (hdr.Size >= 1<<32)

	// box size + name
	if largeSize {
		binary.BigEndian.PutUint32(buf[0:boxSizeLen], 1) // signals large size
	} else {
		binary.BigEndian.PutUint32(buf[0:boxSizeLen], uint32(hdr.Size))
	}
	n = boxSizeLen
	nc := copy(buf[boxSizeLen:boxSizeLen+boxNameLen], hdr.Name[:boxNameLen])
	if nc != boxNameLen {
		return n, io.ErrShortWrite
	}
	n += boxNameLen

	// box large size
	if largeSize {
		binary.BigEndian.PutUint64(buf[n:n+largeSizeLen], hdr.Size)
		n += largeSizeLen
	}

	return n, nil
}

const (
	Avc1BoxStr = "avc1"
	Avc3BoxStr = "avc3"
	AvcCBoxStr = "avcC"
	BtrtBoxStr = "btrt"
	CdatBoxStr = "cdat"
	CdscBoxStr = "cdsc"
	ClapBoxStr = "clap"
	Co64BoxStr = "co64"
	CslgBoxStr = "cslg"
	CtimBoxStr = "ctim"
	CTooBoxStr = "\xa9too"
	CttsBoxStr = "ctts"
	DataBoxStr = "data"
	DinfBoxStr = "dinf"
	DpndBoxStr = "dpnd"
	DrefBoxStr = "dref"
	EdtsBoxStr = "edts"
	ElngBoxStr = "elng"
	ElstBoxStr = "elst"
	EmsgBoxStr = "emsg"
	EncaBoxStr = "enca"
	EncvBoxStr = "encv"
	EsdsBoxStr = "esds"
	FontBoxStr = "font"
	FreeBoxStr = "free"
	FrmaBoxStr = "frma"
	FtypBoxStr = "ftyp"
	HdlrBoxStr = "hdlr"
	Hev1BoxStr = "hev1"
	HindBoxStr = "hind"
	HintBoxStr = "hint"
	Hvc1BoxStr = "hvc1"
	HvcCBoxStr = "hvcC"
	IdenBoxStr = "iden"
	IlstBoxStr = "ilst"
	IodsBoxStr = "iods"
	IpirBoxStr = "ipir"
	KindBoxStr = "kind"
	MdatBoxStr = "mdat"
	MdhdBoxStr = "mdhd"
	MdiaBoxStr = "mdia"
	MehdBoxStr = "mehd"
	MetaBoxStr = "meta"
	MfhdBoxStr = "mfhd"
	MfraBoxStr = "mfra"
	MfroBoxStr = "mfro"
	MimeBoxStr = "mime"
	MinfBoxStr = "minf"
	MoofBoxStr = "moof"
	MoovBoxStr = "moov"
	Mp4aBoxStr = "mp4a"
	MpodBoxStr = "mpod"
	MvexBoxStr = "mvex"
	MvhdBoxStr = "mvhd"
	NmhdBoxStr = "nmhd"
	PaspBoxStr = "pasp"
	PaylBoxStr = "payl"
	PrftBoxStr = "prft"
	PsshBoxStr = "pssh"
	SaioBoxStr = "saio"
	SaizBoxStr = "saiz"
	SbgpBoxStr = "sbgp"
	SchiBoxStr = "schi"
	SchmBoxStr = "schm"
	SdtpBoxStr = "sdtp"
	SencBoxStr = "senc"
	SgpdBoxStr = "sgpd"
	SidxBoxStr = "sidx"
	SinfBoxStr = "sinf"
	SkipBoxStr = "skip"
	SmhdBoxStr = "smhd"
	StblBoxStr = "stbl"
	StcoBoxStr = "stco"
	SthdBoxStr = "sthd"
	StppBoxStr = "stpp"
	StscBoxStr = "stsc"
	StsdBoxStr = "stsd"
	StssBoxStr = "stss"
	StszBoxStr = "stsz"
	SttgBoxStr = "sttg"
	SttsBoxStr = "stts"
	StypBoxStr = "styp"
	SubsBoxStr = "subs"
	SubtBoxStr = "subt"
	SyncBoxStr = "sync"
	TencBoxStr = "tenc"
	TfdtBoxStr = "tfdt"
	TfhdBoxStr = "tfhd"
	TfraBoxStr = "tfra"
	TkhdBoxStr = "tkhd"
	TrafBoxStr = "traf"
	TrakBoxStr = "trak"
	TrefBoxStr = "tref"
	TrepBoxStr = "trep"
	TrexBoxStr = "trex"
	TrunBoxStr = "trun"
	UdtaBoxStr = "udta"
	UrlBoxStr  = "url "
	UuidBoxStr = "uuid"
	VdepBoxStr = "vdep"
	VlabBoxStr = "vlab"
	VmhdBoxStr = "vmhd"
	VplxBoxStr = "vplx"
	VsidBoxStr = "vsid"
	VttaBoxStr = "vtta"
	VttcBoxStr = "vttc"
	VttCBoxStr = "vttC"
	VtteBoxStr = "vtte"
	WvttBoxStr = "wvtt"
)
