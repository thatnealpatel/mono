package ranking

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

// IDF binary wire format, version 1.
//
// Byte Order: Little Endian
//
// ┌────────────────────────────────────────────┐
// │  magic       [16 bytes]                    │◄── "IDF\0" + 13 zero bytes; fixed-width
// │  version     uint8       [1 byte]          │◄── forward compat; reader rejects unknown
// ├────────────────────────────────────────────┤
// │  term_count  uint32      [4 bytes]         │◄── number of terms with posting lists
// ├────────────────────────────────────────────┤
// │                                            │
// │  ┌────────────────────────────────────────┐│
// │  │  key_len      uint16  [2 bytes]        ││◄── term string length
// │  │  key          []byte  [key_len]        ││◄── tokenized term, raw bytes
// │  │  idf          float32 [4 bytes]        ││◄── inverse document frequency
// │  │  posting_len  uint32  [4 bytes]        ││◄── number of doc IDs in posting list
// │  │  postings     []int32 [4*posting_len]  ││◄── contiguous doc ID array
// │  └────────────────────────────────────────┘│
// │  ... repeated term_count times             │
// │                                            │
// └────────────────────────────────────────────┘

var idfMagic = [16]byte{'I', 'D', 'F'}

const idfVersion = 1

func idfWriteBinary(idx *IDF, w io.Writer) (int64, error) {
	cw := &countWriter{w: w}
	bw := bufio.NewWriter(cw)
	if _, err := bw.Write(idfMagic[:]); err != nil {
		return cw.n, err
	}
	if err := bw.WriteByte(idfVersion); err != nil {
		return cw.n, err
	}
	if err := writeU32(bw, uint32(len(idx.postings))); err != nil {
		return cw.n, err
	}
	for term, pl := range idx.postings {
		if err := writeU16(bw, uint16(len(term))); err != nil {
			return cw.n, err
		}
		if _, err := bw.WriteString(term); err != nil {
			return cw.n, err
		}
		if err := writeF32(bw, float32(idx.idf[term])); err != nil {
			return cw.n, err
		}
		if err := writeU32(bw, uint32(len(pl))); err != nil {
			return cw.n, err
		}
		for _, p := range pl {
			if err := writeI32(bw, p.Doc); err != nil {
				return cw.n, err
			}
		}
	}
	return cw.n, bw.Flush()
}

func idfReadBinary(idx *IDF, r io.Reader) (int64, error) {
	cr := &countReader{r: r}
	br := bufio.NewReader(cr)
	var magic [16]byte
	if _, err := io.ReadFull(br, magic[:]); err != nil {
		return cr.n, err
	}
	if magic != idfMagic {
		return cr.n, fmt.Errorf("idf: bad magic")
	}
	version, err := br.ReadByte()
	if err != nil {
		return cr.n, err
	}
	if version != idfVersion {
		return cr.n, fmt.Errorf("idf: unsupported version %d", version)
	}
	termCount, err := readU32(br)
	if err != nil {
		return cr.n, err
	}
	idx.postings = make(map[string][]idfPosting, termCount)
	idx.idf = make(map[string]float64, termCount)
	for range termCount {
		keyLen, err := readU16(br)
		if err != nil {
			return cr.n, err
		}
		key := make([]byte, keyLen)
		if _, err := io.ReadFull(br, key); err != nil {
			return cr.n, err
		}
		idf, err := readF32(br)
		if err != nil {
			return cr.n, err
		}
		postingLen, err := readU32(br)
		if err != nil {
			return cr.n, err
		}
		pl := make([]idfPosting, postingLen)
		for i := range pl {
			doc, err := readI32(br)
			if err != nil {
				return cr.n, err
			}
			pl[i] = idfPosting{Doc: doc}
		}
		term := string(key)
		idx.postings[term] = pl
		idx.idf[term] = float64(idf)
	}
	return cr.n, nil
}

var le = binary.LittleEndian

func writeF32(w io.Writer, v float32) error {
	var buf [4]byte
	le.PutUint32(buf[:], math.Float32bits(v))
	_, err := w.Write(buf[:])
	return err
}

func writeU32(w io.Writer, v uint32) error {
	var buf [4]byte
	le.PutUint32(buf[:], v)
	_, err := w.Write(buf[:])
	return err
}

func writeU16(w io.Writer, v uint16) error {
	var buf [2]byte
	le.PutUint16(buf[:], v)
	_, err := w.Write(buf[:])
	return err
}

func writeI32(w io.Writer, v int32) error {
	var buf [4]byte
	le.PutUint32(buf[:], uint32(v))
	_, err := w.Write(buf[:])
	return err
}

func readF32(r io.Reader) (float32, error) {
	var buf [4]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return 0, err
	}
	return math.Float32frombits(le.Uint32(buf[:])), nil
}

func readU32(r io.Reader) (uint32, error) {
	var buf [4]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return 0, err
	}
	return le.Uint32(buf[:]), nil
}

func readU16(r io.Reader) (uint16, error) {
	var buf [2]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return 0, err
	}
	return le.Uint16(buf[:]), nil
}

func readI32(r io.Reader) (int32, error) {
	var buf [4]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return 0, err
	}
	return int32(le.Uint32(buf[:])), nil
}
