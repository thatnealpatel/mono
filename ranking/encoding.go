package ranking

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

// BM25 binary wire format, version 1.
//
// Byte Order: Little Endian
//
// ┌────────────────────────────────────────────┐
// │  magic       [16 bytes]                    │◄── "BM25" + 12 zero bytes; fixed-width
// │  version     uint8       [1 byte]          │◄── forward compat; reader rejects unknown
// ├────────────────────────────────────────────┤
// │  K1          float32     [4 bytes]         │◄── scoring params the index was built with;
// │  B           float32     [4 bytes]         │    must match at query time
// ├────────────────────────────────────────────┤
// │  doc_count   uint32      [4 bytes]         │◄── pre-allocate slice, bounds-check on read
// ├────────────────────────────────────────────┤
// │                                            │
// │  ┌────────────────────────────────────────┐│
// │  │  DL        float32   [4 bytes]         ││◄── document length for normalization
// │  │  tf_count  uint16    [2 bytes]         ││◄── unique terms per doc; uint16 sufficient
// │  │                                        ││
// │  │  ┌────────────────────────────────────┐││
// │  │  │  key_len  uint16  [2 bytes]        │││◄── individual term < 64KB
// │  │  │  key      []byte  [key_len]        │││◄── tokenized term, raw bytes
// │  │  │  tf       float32 [4 bytes]        │││◄── term frequency in this doc
// │  │  └────────────────────────────────────┘││
// │  │  ... repeated tf_count times           ││
// │  └────────────────────────────────────────┘│
// │  ... repeated doc_count times              │
// │                                            │
// └────────────────────────────────────────────┘
//
// On read: avgdl and IDF are recomputed from the loaded data.

var bm25Magic = [16]byte{'B', 'M', '2', '5'}

const bm25Version = 1

func bm25WriteTo(bm *BM25, w io.Writer) (int64, error) {
	cw := &countWriter{w: w}
	bw := bufio.NewWriter(cw)
	if _, err := bw.Write(bm25Magic[:]); err != nil {
		return cw.n, err
	}
	if err := bw.WriteByte(bm25Version); err != nil {
		return cw.n, err
	}
	if err := writeF32(bw, float32(bm.k1)); err != nil {
		return cw.n, err
	}
	if err := writeF32(bw, float32(bm.b)); err != nil {
		return cw.n, err
	}
	if err := writeU32(bw, uint32(len(bm.docs))); err != nil {
		return cw.n, err
	}
	for _, doc := range bm.docs {
		if err := writeF32(bw, float32(doc.DL)); err != nil {
			return cw.n, err
		}
		if err := writeU16(bw, uint16(len(doc.TF))); err != nil {
			return cw.n, err
		}
		for term, tf := range doc.TF {
			if err := writeU16(bw, uint16(len(term))); err != nil {
				return cw.n, err
			}
			if _, err := bw.WriteString(term); err != nil {
				return cw.n, err
			}
			if err := writeF32(bw, float32(tf)); err != nil {
				return cw.n, err
			}
		}
	}
	return cw.n, bw.Flush()
}

func bm25ReadFrom(bm *BM25, r io.Reader) (int64, error) {
	cr := &countReader{r: r}
	br := bufio.NewReader(cr)
	var magic [16]byte
	if _, err := io.ReadFull(br, magic[:]); err != nil {
		return cr.n, err
	}
	if magic != bm25Magic {
		return cr.n, fmt.Errorf("bm25: bad magic")
	}
	version, err := br.ReadByte()
	if err != nil {
		return cr.n, err
	}
	if version != bm25Version {
		return cr.n, fmt.Errorf("bm25: unsupported version %d", version)
	}
	k1, err := readF32(br)
	if err != nil {
		return cr.n, err
	}
	b, err := readF32(br)
	if err != nil {
		return cr.n, err
	}
	docCount, err := readU32(br)
	if err != nil {
		return cr.n, err
	}
	bm.k1 = float64(k1)
	bm.b = float64(b)
	bm.docs = make([]bm25Doc, docCount)
	docFreq := map[string]int{}
	var totalLen float64
	for i := range bm.docs {
		dl, err := readF32(br)
		if err != nil {
			return cr.n, err
		}
		tfCount, err := readU16(br)
		if err != nil {
			return cr.n, err
		}
		tf := make(map[string]float64, tfCount)
		for range tfCount {
			keyLen, err := readU16(br)
			if err != nil {
				return cr.n, err
			}
			key := make([]byte, keyLen)
			if _, err := io.ReadFull(br, key); err != nil {
				return cr.n, err
			}
			v, err := readF32(br)
			if err != nil {
				return cr.n, err
			}
			term := string(key)
			tf[term] = float64(v)
			docFreq[term]++
		}
		bm.docs[i] = bm25Doc{TF: tf, DL: float64(dl)}
		totalLen += float64(dl)
	}
	n := float64(docCount)
	if n > 0 {
		bm.avgdl = totalLen / n
	}
	bm.idf = make(map[string]float64, len(docFreq))
	for term, df := range docFreq {
		bm.idf[term] = math.Log((n-float64(df)+0.5)/(float64(df)+0.5) + 1)
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

type countWriter struct {
	w io.Writer
	n int64
}

func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

type countReader struct {
	r io.Reader
	n int64
}

func (c *countReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}
