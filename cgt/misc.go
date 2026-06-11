package cgt

import (
	"fmt"
	"math/bits"
	"math/rand/v2"
)

// p3Incidences[x] is a bitmap of the nodes
// incident with node x. Bit y is set iff
// node y is incident with node x.
//
// p3IncLists[x] is the sorted list of the
// four nodes incident with node x.
//
// We number the 13 points 0..12 and the 13
// lines 13..25. Point i and line j are
// incident iff i + j is 0, 1, 3, or 9 mod 13.
var (
	p3Incidences [26]uint32
	p3IncLists   [26][4]int
)

func init() {
	for x := 0; x < 13; x++ {
		var blist uint32
		for _, p := range [...]int{0, 1, 3, 9} {
			blist |= 1 << uint(((p-x)%13+13)%13)
		}
		p3Incidences[x] = blist << 13
		p3Incidences[x+13] = blist
		p3IncLists[x] = bitList4(p3Incidences[x])
		p3IncLists[x+13] = bitList4(p3Incidences[x+13])
	}
}

// bitList4 returns the four set bit positions
// of m in ascending order. bitList4 panics if
// m does not have exactly four set bits.
func bitList4(m uint32) [4]int {
	var out [4]int
	n := 0
	for m != 0 {
		b := bits.TrailingZeros32(m)
		if n == 4 {
			panic("cgt: bitList4 needs exactly 4 bits")
		}
		out[n] = b
		n++
		m &= m - 1
	}
	if n != 4 {
		panic("cgt: bitList4 needs exactly 4 bits")
	}
	return out
}

// bitList returns the set bit positions of m
// in ascending order.
func bitList(m uint32) []int {
	out := make([]int, 0, bits.OnesCount32(m))
	for m != 0 {
		out = append(out, bits.TrailingZeros32(m))
		m &= m - 1
	}
	return out
}

// P3Node models a point or a line in the
// projective plane PG(2,3). Points are
// numbered 0..12 and lines 13..25.
type P3Node struct {
	ord int
}

// p3Obj converts obj to the number of a P3
// node. It accepts an int in 0..25, a P3Node,
// or a string name. Recognised strings are the
// decimal forms "0".."25", the point names
// "P0".."P12", and the line names "L0".."L12".
// p3Obj panics on any other value.
func p3Obj(obj any) int {
	switch v := obj.(type) {
	case P3Node:
		return v.ord
	case int:
		if v < 0 || v >= 26 {
			panic("cgt: P3 node number out of range")
		}
		return v
	case string:
		if n, ok := p3Name(v); ok {
			return n
		}
		panic(fmt.Sprintf("cgt: cannot convert string %q to P3 node", v))
	default:
		panic(fmt.Sprintf("cgt: cannot convert %T to P3 node", obj))
	}
}

// p3Name maps a node name to its number. It
// returns false if the name is unknown.
func p3Name(s string) (int, bool) {
	if n, err := atoiP3(s); err == nil {
		if n >= 0 && n < 26 {
			return n, true
		}
		return 0, false
	}
	if len(s) >= 2 && (s[0] == 'P' || s[0] == 'L') {
		n, err := atoiP3(s[1:])
		if err != nil || n < 0 || n >= 13 {
			return 0, false
		}
		if s[0] == 'L' {
			n += 13
		}
		return n, true
	}
	return 0, false
}

// atoiP3 parses a non-negative decimal string.
// atoiP3 returns an error for any non-digit
// input or empty string.
func atoiP3(s string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("cgt: empty number")
	}
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("cgt: bad digit %q", c)
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

// p3List converts obj to a list of P3 node
// numbers. A slice or array is mapped element
// by element; a comma-separated string is split
// first; anything else is treated as a single
// node.
func p3List(obj any) []int {
	switch v := obj.(type) {
	case []int:
		out := make([]int, len(v))
		for i, x := range v {
			out[i] = p3Obj(x)
		}
		return out
	case []any:
		out := make([]int, len(v))
		for i, x := range v {
			out[i] = p3Obj(x)
		}
		return out
	case string:
		var out []int
		for _, part := range splitComma(v) {
			out = append(out, p3Obj(part))
		}
		return out
	default:
		return []int{p3Obj(v)}
	}
}

// splitComma splits s on commas and trims
// surrounding whitespace, dropping empty
// fields.
func splitComma(s string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			f := trimSpace(s[start:i])
			if f != "" {
				out = append(out, f)
			}
			start = i + 1
		}
	}
	return out
}

// trimSpace trims ASCII spaces and tabs from
// both ends of s.
func trimSpace(s string) string {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r') {
		i++
	}
	j := len(s)
	for j > i && (s[j-1] == ' ' || s[j-1] == '\t' || s[j-1] == '\n' || s[j-1] == '\r') {
		j--
	}
	return s[i:j]
}

// NewP3Node returns the P3 node described by
// obj. NewP3Node panics if obj is not a valid
// node description (see p3Obj).
func NewP3Node(obj any) P3Node {
	return P3Node{ord: p3Obj(obj)}
}

// Ord returns the internal node number.
func (p P3Node) Ord() int {
	return p.ord
}

// Name returns the node name, "P0".."P12" for
// points and "L0".."L12" for lines.
func (p P3Node) Name() string {
	q, r := p.ord/13, p.ord%13
	return string("PL"[q]) + itoaP3(r)
}

// itoaP3 formats a non-negative int as decimal.
func itoaP3(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// Equal reports whether p and other are the
// same node.
func (p P3Node) Equal(other P3Node) bool {
	return p.ord == other.ord
}

// Apply returns the image of p under the
// automorphism g. A point i maps to the point
// g.perm[i]. A line maps to the unique line
// through the images of its first two points.
func (p P3Node) Apply(g *AutP3) P3Node {
	if p.ord < 13 {
		return P3Node{ord: g.perm[p.ord]}
	}
	pts := p3IncLists[p.ord]
	im1, im2 := g.perm[pts[0]], g.perm[pts[1]]
	return P3Incidence(im1, im2)
}

// P3Incidence returns the unique P3 node
// incident with all the given nodes. Its
// typical use is the line through two points
// or the intersection of two lines.
// P3Incidence panics if no such node exists or
// it is not unique.
func P3Incidence(nodes ...any) P3Node {
	a := uint32(0x3ffffff)
	for _, n := range nodes {
		a &= p3Incidences[p3Obj(n)]
	}
	if bits.OnesCount32(a) == 1 {
		return P3Node{ord: bits.TrailingZeros32(a)}
	}
	if a != 0 {
		panic("cgt: incident P3 node is not unique")
	}
	panic("cgt: no incident P3 node found")
}

// P3Incidences returns the sorted list of P3
// nodes incident with at least one node in
// each argument set and not contained in any
// of those sets. Each argument is a set of
// nodes: an int is a singleton, a slice is a
// set, and a comma-separated string is a set.
func P3Incidences(nodes ...any) []P3Node {
	all := uint32(0x3ffffff)
	noNodes := uint32(0)
	for _, arg := range nodes {
		if x, ok := arg.(int); ok {
			n := p3Obj(x)
			all &= p3Incidences[n]
			noNodes |= 1 << uint(n)
			continue
		}
		var union uint32
		for _, n := range p3List(arg) {
			union |= p3Incidences[n]
			noNodes |= 1 << uint(n)
		}
		all &= union
	}
	res := bitList(all &^ noNodes)
	out := make([]P3Node, len(res))
	for i, n := range res {
		out[i] = P3Node{ord: n}
	}
	return out
}

// remainingNodes returns the two nodes that,
// together with x1 and x2, complete the line
// through x1 and x2 (or the pencil of lines
// meeting in their common point). remainingNodes
// panics if x1 and x2 are equal or are not both
// points or both lines.
func remainingNodes(x1, x2 int) []int {
	common := bitList(p3Incidences[x1] & p3Incidences[x2])
	if len(common) == 1 {
		rem := p3Incidences[common[0]] &^ ((1 << uint(x1)) | (1 << uint(x2)))
		return bitList(rem)
	}
	if len(common) != 0 {
		panic("cgt: P3RemainingNodes arguments must differ")
	}
	panic("cgt: P3RemainingNodes nodes must all be points or all lines")
}

// P3RemainingNodes returns the two remaining
// nodes on the line through points x1, x2, or
// the two remaining lines through the common
// point of lines x1, x2. P3RemainingNodes
// panics if x1 and x2 are equal or are not both
// points or both lines.
func P3RemainingNodes(x1, x2 int) []P3Node {
	rem := remainingNodes(p3Obj(x1), p3Obj(x2))
	out := make([]P3Node, len(rem))
	for i, n := range rem {
		out[i] = P3Node{ord: n}
	}
	return out
}

// findCollinear searches points for three
// collinear points. On success it returns the
// triple plus the fourth point of their line;
// otherwise it returns nil. points must be
// distinct points (numbers 0..12).
func findCollinear(points []int) []int {
	if len(points) < 3 {
		return nil
	}
	if len(points) > 5 {
		points = points[:5]
	}
	for i1, x1 := range points {
		for _, x2 := range points[i1+1:] {
			rem := remainingNodes(x1, x2)
			x3, x4 := rem[0], rem[1]
			if containsInt(points, x3) {
				return []int{x1, x2, x3, x4}
			}
			if containsInt(points, x4) {
				return []int{x1, x2, x4, x3}
			}
		}
	}
	return nil
}

// containsInt reports whether s contains x.
func containsInt(s []int, x int) bool {
	for _, v := range s {
		if v == x {
			return true
		}
	}
	return false
}

// P3IsCollinear reports whether the given set
// of nodes contains three collinear points or
// three collinear lines. Each argument is a
// node description as in NewP3Node.
func P3IsCollinear(points []int) bool {
	var bitmap uint32
	for _, p := range points {
		bitmap |= 1 << uint(p3Obj(p))
	}
	if findCollinear(bitList(bitmap&0x1fff)) != nil {
		return true
	}
	if findCollinear(bitList(bitmap>>13)) != nil {
		return true
	}
	return false
}

// P3PointSetType returns an automorphism
// invariant of a set of points. The first
// returned value is the number of points; the
// rest counts, for k = 0..4, how many of the
// 13 lines meet the set in exactly k points.
// P3PointSetType panics if s contains a value
// outside 0..12.
func P3PointSetType(s []int) int {
	var bl uint32
	for _, p := range s {
		if p < 0 || p >= 13 {
			panic("cgt: P3PointSetType point out of range")
		}
		bl |= 1 << uint(p)
	}
	var l [5]int
	for i := 13; i < 26; i++ {
		l[bits.OnesCount32(p3Incidences[i]&bl)]++
	}
	// Pack weight and the five counts into a
	// single invariant int: weight in the top
	// digits, then l[0]..l[4] each in 4 bits.
	w := bits.OnesCount32(bl)
	inv := w
	for _, c := range l {
		inv = inv<<4 | c
	}
	return inv
}

// AutP3 models an automorphism of the
// projective plane PG(2,3) as a permutation of
// its 13 points.
type AutP3 struct {
	perm []int
}

// fstCross is a fixed cross: four points of
// PG(2,3), no three of them collinear.
var fstCross = [4]int{0, 1, 2, 5}

// commonLinePoints returns the bitmap of all
// points on the line through points x1 and x2.
func commonLinePoints(x1, x2 int) uint32 {
	bl := bitList(p3Incidences[x1] & p3Incidences[x2])
	if len(bl) != 1 {
		panic("cgt: P3 points do not span a unique line")
	}
	return p3Incidences[bl[0]]
}

// findCross returns a cross (four points, no
// three collinear) contained in points, or nil
// if none exists. points must be distinct
// points. Any six points contain a cross.
func findCross(points []int) []int {
	if len(points) > 6 {
		points = points[:6]
	}
	n := len(points)
	if n < 4 {
		return nil
	}
	for i1 := 0; i1 < n; i1++ {
		x1 := points[i1]
		for i2 := i1 + 1; i2 < n; i2++ {
			x2 := points[i2]
			s12 := commonLinePoints(x1, x2)
			for i3 := i2 + 1; i3 < n; i3++ {
				x3 := points[i3]
				if s12&(1<<uint(x3)) != 0 {
					continue
				}
				s123 := s12 | commonLinePoints(x1, x3) | commonLinePoints(x2, x3)
				for i4 := i3 + 1; i4 < n; i4++ {
					x4 := points[i4]
					if s123&(1<<uint(x4)) == 0 {
						return []int{x1, x2, x3, x4}
					}
				}
			}
		}
	}
	return nil
}

// crossIntersection returns [y, y1, y2] where
// y is the intersection of the line through
// x11, x12 and the line through x21, x22, and
// y1, y2 are the remaining points on those two
// lines. crossIntersection panics if three of
// the four points are collinear.
func crossIntersection(x11, x12, x21, x22 int) (int, int, int) {
	remain := func(a, b int) uint32 {
		bl := bitList(p3Incidences[a] & p3Incidences[b])
		if len(bl) != 1 {
			panic("cgt: P3 points do not span a unique line")
		}
		return p3Incidences[bl[0]] &^ ((1 << uint(a)) | (1 << uint(b)))
	}
	s1 := remain(x11, x12)
	s2 := remain(x21, x22)
	if s1 == s2 || s1&s2 == 0 {
		panic("cgt: collinear points in crossIntersection")
	}
	return bits.TrailingZeros32(s1 & s2),
		bits.TrailingZeros32(s1 &^ s2),
		bits.TrailingZeros32(s2 &^ s1)
}

// mapCross returns the unique automorphism (as
// a permutation of the 13 points) mapping
// cross1 to cross2. Both arguments must be
// crosses, i.e. four non-collinear points.
func mapCross(cross1, cross2 [4]int) []int {
	perm := make([]int, 13)
	c1 := []int{cross1[0] % 13, cross1[1] % 13, cross1[2] % 13, cross1[3] % 13}
	c2 := []int{cross2[0] % 13, cross2[1] % 13, cross2[2] % 13, cross2[3] % 13}
	for i := 0; i < 3; i++ {
		y0, y1, y2 := crossIntersection(c1[0], c1[1], c1[2], c1[3])
		c1 = append(c1, y0, y1, y2)
		z0, z1, z2 := crossIntersection(c2[0], c2[1], c2[2], c2[3])
		c2 = append(c2, z0, z1, z2)
		c1[0], c1[1], c1[2] = c1[1], c1[2], c1[0]
		c2[0], c2[1], c2[2] = c2[1], c2[2], c2[0]
	}
	for i := 0; i < 13; i++ {
		perm[c1[i]] = c2[i]
	}
	return perm
}

// lineMapFromMap converts a point permutation
// perm to the corresponding line permutation.
// Entry i of the result is the image of line i,
// reduced modulo 13. By the point/line duality
// the same routine converts a line permutation
// to a point permutation.
func lineMapFromMap(perm []int) []int {
	out := make([]int, 13)
	for x := 13; x < 26; x++ {
		pts := p3IncLists[x]
		img := p3Incidences[perm[pts[0]]] & p3Incidences[perm[pts[1]]]
		out[x-13] = bits.TrailingZeros32(img >> 13)
	}
	return out
}

// mapP3ToPerm finds an automorphism compatible
// with the partial point mapping obj1[i] ->
// obj2[i]. If unique is true it requires the
// mapping to determine a unique automorphism;
// otherwise it fills the mapping out randomly.
// All entries must be points or all lines.
// mapP3ToPerm panics if no compatible
// automorphism exists.
func mapP3ToPerm(obj1, obj2 []int, unique bool) []int {
	line := 0
	if len(obj1)+len(obj2) > 0 {
		all := append(append([]int{}, obj1...), obj2...)
		mn, mx := all[0]/13, all[0]/13
		for _, x := range all {
			d := x / 13
			if d < mn {
				mn = d
			}
			if d > mx {
				mx = d
			}
		}
		line = mn
		if !(mn == mx && (mn == 0 || mn == 1)) {
			panic("cgt: P3 mapping must be all points or all lines")
		}
	}
	if len(obj1) != len(obj2) {
		panic("cgt: P3 mapping preimage and image differ in length")
	}
	a1 := make([]int, len(obj1))
	a2 := make([]int, len(obj2))
	for i := range obj1 {
		a1[i] = obj1[i] % 13
		a2[i] = obj2[i] % 13
	}
	var cross1, cross2 [4]int
	if unique {
		c1 := findCross(a1)
		if c1 == nil {
			panic("cgt: P3 mapping is underdetermined")
		}
		c2 := findCross(a2)
		if c2 == nil {
			panic("cgt: P3 mapping is underdetermined")
		}
		copy(cross1[:], c1)
		copy(cross2[:], c2)
	} else {
		copy(cross1[:], completeCrossRandom(a1))
		copy(cross2[:], completeCrossRandom(a2))
	}
	perm := mapCross(cross1, cross2)
	for i, p1 := range a1 {
		if perm[p1] != a2[i] {
			panic("cgt: mapping does not preserve P3")
		}
	}
	if line != 0 {
		perm = lineMapFromMap(perm)
	}
	return perm
}

// completeCrossRandom returns a cross contained
// in points, extending points with random
// points where necessary. points must be
// distinct points.
func completeCrossRandom(points []int) []int {
	pts := append([]int{}, points...)
	if len(pts) == 0 {
		pts = []int{rand.IntN(13)}
	}
	if cross := findCross(pts); cross != nil {
		return cross
	}
	if len(pts) > 6 {
		pts = pts[:6]
	}
	line := findCollinear(pts)
	if line != nil {
		others := setDiff(pts, line)
		var y1 int
		if len(others) == 0 {
			cand := setDiff(intRange(13), line)
			y1 = cand[rand.IntN(len(cand))]
		} else {
			y1 = others[0]
		}
		rem := remainingNodes(y1, line[2])
		y2 := rem[rand.IntN(len(rem))]
		return []int{line[0], line[1], y1, y2}
	}
	others := setDiff(intRange(13), pts)
	if len(pts) < 2 {
		need := 2 - len(pts)
		pick := sampleN(others, need)
		pts = append(pts, pick...)
		others = setDiff(others, pts)
	}
	others = setDiff(others, remainingNodes(pts[0], pts[1]))
	if len(pts) < 3 {
		x := others[rand.IntN(len(others))]
		pts = append(pts, x)
		others = setDiff(others, []int{x})
	}
	others = setDiff(others, remainingNodes(pts[0], pts[2]))
	others = setDiff(others, remainingNodes(pts[1], pts[2]))
	pts = append(pts, others[rand.IntN(len(others))])
	return pts
}

// intRange returns the slice 0, 1, ..., n-1.
func intRange(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}

// setDiff returns the elements of a that are
// not in b, preserving the order of a.
func setDiff(a, b []int) []int {
	var out []int
	for _, x := range a {
		if !containsInt(b, x) {
			out = append(out, x)
		}
	}
	return out
}

// sampleN returns n distinct random elements of
// s. sampleN panics if n exceeds len(s).
func sampleN(s []int, n int) []int {
	if n > len(s) {
		panic("cgt: sampleN n too large")
	}
	pool := append([]int{}, s...)
	rand.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	return pool[:n]
}

// checkPermP3 verifies that perm (a length-13
// list) is an automorphism of PG(2,3) and
// returns it reduced modulo 13. checkPermP3
// panics if perm is not an automorphism.
func checkPermP3(perm []int) []int {
	if len(perm) != 13 {
		panic("cgt: P3 point permutation must have length 13")
	}
	red := make([]int, 13)
	for i, x := range perm {
		red[i] = ((x % 13) + 13) % 13
	}
	var imgCross [4]int
	for i, c := range fstCross {
		imgCross[i] = red[c]
	}
	imgPerm := mapP3ToPerm(fstCross[:], imgCross[:], true)
	for i := range red {
		if red[i] != imgPerm[i] {
			panic("cgt: mapping does not preserve P3")
		}
	}
	return red
}

// invertPermP3 returns the inverse of a point
// permutation of PG(2,3).
func invertPermP3(perm []int) []int {
	inv := make([]int, 13)
	for i, x := range perm {
		inv[x] = i
	}
	return inv
}

// mulPermP3 returns the product perm1 * perm2
// of two point permutations: i -> perm2[perm1[i]].
func mulPermP3(perm1, perm2 []int) []int {
	out := make([]int, 13)
	for i, x := range perm1 {
		out[i] = perm2[x]
	}
	return out
}

// p3Mapping builds the point permutation of an
// AutP3 from a constructor argument. A nil
// argument yields the identity. A map describes
// a partial mapping of points or lines. If
// random is true an unconstrained or partially
// constrained random automorphism is built.
func p3Mapping(src any, random bool) []int {
	if src == nil {
		if !random {
			return intRange(13)
		}
		src = map[any]any{0: rand.IntN(13)}
	}
	if s, ok := src.(string); ok {
		m := map[any]any{}
		for _, pair := range splitComma(s) {
			kv := splitColon(pair)
			if len(kv) != 2 {
				panic("cgt: cannot parse P3 mapping string")
			}
			m[trimSpace(kv[0])] = trimSpace(kv[1])
		}
		src = m
	}
	keys, vals := mapItems(src)
	return mapP3ToPerm(p3List(keys), p3List(vals), !random)
}

// splitColon splits s on its first colon.
func splitColon(s string) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == ':' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}

// mapItems extracts the keys and values of a
// mapping argument. It accepts map[any]any,
// map[int]int, and map[string]string.
// mapItems panics on any other type.
func mapItems(src any) (keys, vals []any) {
	switch m := src.(type) {
	case map[any]any:
		for k, v := range m {
			keys = append(keys, k)
			vals = append(vals, v)
		}
	case map[int]int:
		for k, v := range m {
			keys = append(keys, k)
			vals = append(vals, v)
		}
	case map[string]string:
		for k, v := range m {
			keys = append(keys, k)
			vals = append(vals, v)
		}
	default:
		panic(fmt.Sprintf("cgt: cannot build AutP3 from %T", src))
	}
	return keys, vals
}

// NewAutP3 returns the automorphism of PG(2,3)
// described by mapping. A nil mapping yields
// the identity. An *AutP3 is copied. A map
// gives a partial mapping of points or lines.
// The string "p" or "l" cannot be passed here;
// use a 13-element point or line list via a map
// instead. NewAutP3 panics if the mapping does
// not extend to a unique automorphism.
func NewAutP3(mapping any) *AutP3 {
	switch m := mapping.(type) {
	case nil:
		return &AutP3{perm: intRange(13)}
	case *AutP3:
		return &AutP3{perm: append([]int{}, m.perm...)}
	case []int:
		return &AutP3{perm: checkPermP3(m)}
	default:
		return &AutP3{perm: p3Mapping(mapping, false)}
	}
}

// NewAutP3Rand returns a uniformly random
// automorphism of PG(2,3).
func NewAutP3Rand() *AutP3 {
	return &AutP3{perm: p3Mapping(nil, true)}
}

// neutralPermP3 is the identity point
// permutation of PG(2,3).
var neutralPermP3 = []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}

// Order returns the order of g in AutP3.
// Order panics if no order in 1..13 is found.
func (g *AutP3) Order() int {
	if intsEqual(g.perm, neutralPermP3) {
		return 1
	}
	pwr := append([]int{}, g.perm...)
	for o := 2; o <= 13; o++ {
		pwr = mulPermP3(pwr, g.perm)
		if intsEqual(pwr, neutralPermP3) {
			return o
		}
	}
	panic("cgt: cannot compute order in AutP3")
}

// intsEqual reports whether a and b are equal.
func intsEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Mul returns the product g * other.
func (g *AutP3) Mul(other *AutP3) *AutP3 {
	return &AutP3{perm: checkPermP3(mulPermP3(g.perm, other.perm))}
}

// Perm returns the point permutation as a
// length-13 list (a copy).
func (g *AutP3) Perm() []int {
	return append([]int{}, g.perm...)
}

// Inv returns the inverse of g.
func (g *AutP3) Inv() *AutP3 {
	return &AutP3{perm: checkPermP3(invertPermP3(g.perm))}
}

// Pow returns g raised to the power e. The
// exponent may be negative.
func (g *AutP3) Pow(e int) *AutP3 {
	if e < 0 {
		return g.Inv().Pow(-e)
	}
	res := &AutP3{perm: intRange(13)}
	base := g
	for e > 0 {
		if e&1 == 1 {
			res = res.Mul(base)
		}
		e >>= 1
		if e > 0 {
			base = base.Mul(base)
		}
	}
	return res
}

// Equal reports whether g and other are the
// same automorphism.
func (g *AutP3) Equal(other *AutP3) bool {
	return intsEqual(g.perm, other.perm)
}

// PointMap returns the automorphism as a
// length-13 permutation of points (a copy).
func (g *AutP3) PointMap() []int {
	return append([]int{}, g.perm...)
}

// LineMap returns the automorphism as a
// length-13 permutation of lines, each entry
// reduced modulo 13.
func (g *AutP3) LineMap() []int {
	return lineMapFromMap(g.perm)
}

// UFindInit initialises table for the
// union-find algorithm over the set
// 0..len(table)-1. Every element starts in its
// own singleton set. UFindInit returns 0 and
// panics if len(table) exceeds 0x40000000.
func UFindInit(t []uint32) int {
	if len(t) > 0x40000000 {
		panic("cgt: union-find table too large")
	}
	for i := range t {
		t[i] = 0x80000000
	}
	return 0
}

// UFindUnion joins the sets containing i and j.
// It returns 1 if two distinct sets were
// merged and 0 if i and j were already in the
// same set. UFindUnion panics if i or j is out
// of range or len(t) exceeds 0x40000000.
func UFindUnion(t []uint32, i, j uint32) int {
	length := uint32(len(t))
	if uint64(len(t)) > 0x40000000 || i >= length || j >= length {
		panic("cgt: union-find argument out of range")
	}
	u1, p1 := ufindRoot(t, length, i)
	u2, p2 := ufindRoot(t, length, j)
	if u1 == u2 {
		return 0
	}
	// Union by rank: p1, p2 hold the root
	// markers, whose low bits encode the rank.
	if p1 <= p2 {
		t[u1] = u2
		if p1 == p2 {
			t[u2]++
		}
	} else {
		t[u2] = u1
	}
	return 1
}

// ufindRoot returns the root u of the tree
// containing n and the root marker p = t[u]
// (with high bit set), performing path halving.
func ufindRoot(t []uint32, length, n uint32) (u, p uint32) {
	u = n
	p = t[u]
	for p&0x80000000 == 0 {
		if p >= length {
			panic("cgt: corrupt union-find table")
		}
		gp := t[p]
		if gp&0x80000000 != 0 {
			return p, gp
		}
		t[u] = gp
		u = gp
		p = t[u]
	}
	return u, p
}

// UFindFind returns the representative of the
// set containing i. UFindFind panics if i is
// out of range or len(t) exceeds 0x40000000.
func UFindFind(t []uint32, i uint32) uint32 {
	length := uint32(len(t))
	if uint64(len(t)) > 0x40000000 || i >= length {
		panic("cgt: union-find argument out of range")
	}
	u := i
	p := t[u]
	for p&0x80000000 == 0 {
		if p >= length {
			panic("cgt: corrupt union-find table")
		}
		gp := t[p]
		if gp&0x80000000 != 0 {
			return p
		}
		t[u] = gp
		u = gp
		p = t[u]
	}
	return u
}

// UFindFindAllMin rewrites table so that each
// set is represented by its smallest element.
// After this call a subsequent UFindFind
// returns that smallest element.
//
// Each entry is then interpreted as: if the
// high bit is set, the representative of i is i
// itself; otherwise the representative is t[i].
// UFindFindAllMin returns the number of sets in
// the partition and panics if len(t) exceeds
// 0x40000000.
func UFindFindAllMin(t []uint32) int {
	length := uint32(len(t))
	if uint64(len(t)) > 0x40000000 {
		panic("cgt: union-find table too large")
	}
	var res uint32

	// First pass: route every element to its
	// root u; mark u with 0xc0000000 | min(S).
	// Singletons keep their 0x80000000 marker.
	for n := uint32(0); n < length; n++ {
		u, mu := n, n
		p := t[u]
		res += p >> 31
		if p == 0x80000000 {
			continue
		}
		stuck := false
		for p&0x80000000 == 0 {
			if p < mu {
				mu = p
			}
			u = p
			if u >= length {
				u = 0x40000000
				stuck = true
				break
			}
			p = t[u]
		}
		if !stuck {
			if p&0xc0000000 == 0xc0000000 {
				if q := p & 0x3fffffff; q < mu {
					mu = q
				}
			}
			t[u] = mu | 0xc0000000
		}
		u1 := n
		p = t[u1]
		for p&0xc0000000 == 0 {
			t[u1] = u
			u1 = p
			p = t[u1]
		}
	}

	// Second pass: route every element to
	// min(S) and mark min(S) with 0x80000001.
	for n := uint32(0); n < length; n++ {
		u := n
		p := t[u]
		if p&0x80000000 != 0 {
			if p&0x40000000 == 0 {
				continue
			}
			p &= 0x3fffffff
			if p >= length {
				t[u] = 0x40000000
				continue
			}
			t[u] = p
			t[p] = 0x80000001
			continue
		}
		gp := t[p]
		if gp&0x80000000 != 0 {
			if gp&0x40000000 == 0 {
				continue
			}
			gp &= 0x3fffffff
		}
		if gp >= length {
			t[p] = 0x40000000
			continue
		}
		t[u] = gp
		t[p] = gp
		t[gp] = 0x80000001
	}

	return int(res)
}

// UFindPartition writes the partition stored in
// t into data and ind. The i-th set of the
// partition is data[ind[i] : ind[i+1]], with
// each set sorted ascending and the sets
// ordered by their smallest element.
// UFindFindAllMin must be called first.
//
// data must have length len(t) and ind length
// at least nsets+1, where nsets is the number
// of sets. UFindPartition returns nsets and
// panics if the buffers are too small or t is
// inconsistent.
func UFindPartition(t []uint32, data, ind []uint32) int {
	lt := len(t)
	if uint64(lt) > 0x40000000 {
		panic("cgt: union-find table too large")
	}
	if len(data) < lt {
		panic("cgt: union-find data buffer too short")
	}
	lInd := len(ind)
	next := make([]uint32, lt+lInd)

	// Walk the table grouping elements by set.
	// Each set gets an index j; the entries are
	// threaded through ``next`` so that, from
	// index j, both the first and last element
	// of the set are reachable.
	i := 0
	for n := 0; n < lt; n++ {
		p := t[n]
		if p&0x80000000 != 0 {
			if i >= lInd {
				panic("cgt: union-find ind buffer too short")
			}
			ind[i] = uint32(lt + i)
			next[n] = uint32(lt + i)
			next[lt+i] = uint32(n)
			i++
		} else {
			if int(p) >= lt {
				continue
			}
			if int(p) >= n {
				panic("cgt: corrupt union-find table")
			}
			if t[p]&0x80000000 == 0 {
				panic("cgt: corrupt union-find table")
			}
			j := next[p] - uint32(lt)
			if int(j) >= i {
				panic("cgt: corrupt union-find table")
			}
			last := ind[j]
			if next[last] != p {
				panic("cgt: corrupt union-find table")
			}
			ind[j] = uint32(n)
			next[last] = uint32(n)
			next[n] = p
		}
	}

	nsets := i
	if nsets >= lInd {
		panic("cgt: union-find ind buffer too short")
	}

	// Copy each set's elements into data so that
	// members of a set are adjacent, and store
	// the start index of each set in ind.
	n := uint32(0)
	for k := 0; k < nsets; k++ {
		last := ind[k]
		ind[k] = n
		fst := next[last]
		data[n] = fst
		n++
		p := next[lt+k]
		for p != fst {
			data[n] = p
			n++
			p = next[p]
		}
	}
	if int(n) > lt {
		panic("cgt: corrupt union-find table")
	}
	ind[nsets] = n
	return nsets
}

// UFindMakeMap returns a slice mapping each
// element to the smallest element of its set.
// UFindFindAllMin must be called before
// UFindMakeMap. UFindMakeMap panics if t is
// inconsistent or len(t) exceeds 0x40000000.
func UFindMakeMap(t []uint32) []uint32 {
	length := len(t)
	if uint64(length) > 0x40000000 {
		panic("cgt: union-find table too large")
	}
	m := make([]uint32, length)
	for n := 0; n < length; n++ {
		p := t[n]
		switch {
		case p&0x80000000 != 0:
			m[n] = uint32(n)
		case int(p) >= length:
			panic("cgt: corrupt union-find table")
		default:
			if int(p) >= n {
				panic("cgt: corrupt union-find table")
			}
			if t[p]&0x80000000 == 0 {
				panic("cgt: corrupt union-find table")
			}
			m[n] = p
		}
	}
	return m
}

// BitParity returns the parity (0 or 1) of the
// number of set bits in x.
func BitParity(x uint64) int {
	return bits.OnesCount64(x) & 1
}

// BitWeight returns the number of set bits in x.
func BitWeight(x uint64) int {
	return bits.OnesCount64(x)
}

// HadamardSign returns (-1)^parity(i & j), i.e.
// the sign +1 or -1 of the Hadamard matrix
// entry at row i, column j.
func HadamardSign(i, j int) int {
	if BitParity(uint64(i&j)) == 1 {
		return -1
	}
	return 1
}

// ParityHadamardSign returns
// (-1)^(parity(i & j) XOR (parity(i) & parity(j))),
// the entry sign of the parity-adjusted
// Hadamard matrix.
func ParityHadamardSign(i, j int) int {
	e := BitParity(uint64(i&j)) ^ (BitParity(uint64(i)) & BitParity(uint64(j)))
	if e == 1 {
		return -1
	}
	return 1
}

// HadamardTransform multiplies the vector v by
// the normalised 2^k by 2^k Hadamard matrix
// over the field of integers modulo p, where
// 2^k = len(v). Entry j of the result is
//
//	(sum_i v[i] * (-1)^parity(i & j)) * 2^floor(-k/2)  (mod p)
//
// HadamardTransform panics if len(v) is not a
// power of two, p is not an odd prime, or
// 2 is not invertible modulo p.
func HadamardTransform(p int, v []int) []int {
	n := len(v)
	if n == 0 || n&(n-1) != 0 {
		panic("cgt: HadamardTransform length not a power of two")
	}
	if p <= 2 || p&1 == 0 {
		panic("cgt: HadamardTransform modulus must be an odd prime")
	}
	k := bits.TrailingZeros(uint(n))
	// q = 2^floor(-k/2) mod p = inv2^ceil(k/2).
	inv2 := (p + 1) / 2
	q := modPow(int64(inv2), int64((k+1)/2), int64(p))
	out := make([]int, n)
	for j := 0; j < n; j++ {
		var sum int64
		for i := 0; i < n; i++ {
			if BitParity(uint64(i&j)) == 1 {
				sum -= int64(v[i])
			} else {
				sum += int64(v[i])
			}
		}
		r := sum % int64(p)
		r = r * q % int64(p)
		r %= int64(p)
		if r < 0 {
			r += int64(p)
		}
		out[j] = int(r)
	}
	return out
}

// modPow returns base^exp mod m for m > 0 and
// exp >= 0.
func modPow(base, exp, m int64) int64 {
	base %= m
	if base < 0 {
		base += m
	}
	res := int64(1)
	for exp > 0 {
		if exp&1 == 1 {
			res = res * base % m
		}
		exp >>= 1
		base = base * base % m
	}
	return res
}

// XchParity returns a copy of v in which entry
// i is taken from v[len(v)-1-i] when i has odd
// parity and from v[i] when i has even parity.
func XchParity(v []int) []int {
	n := len(v)
	out := make([]int, n)
	for i := 0; i < n; i++ {
		if BitParity(uint64(i)) == 1 {
			out[i] = v[n-i-1]
		} else {
			out[i] = v[i]
		}
	}
	return out
}

// ConjugateInvolutionType returns (I, h) where h
// conjugates the monster involution g to a standard
// representative z (h^-1 g h = z): I = 0 for the
// identity, 1 for a 2A involution (z the involution
// in Q_{x0} with cocode word {2,3}), and 2 for a 2B
// involution (z the central involution of G_{x0}).
//
// ConjugateInvolutionType panics if g is not an
// involution, or if no conjugating element is found
// within the trial budget. It mirrors
// mm_conjugate_involution.
func ConjugateInvolutionType(g *MM) (int, *MM) {
	it, h, ok := conjugateInvolution(g, true, 20)
	if !ok {
		panic("cgt: conjugation of element to central involution failed")
	}
	return it, h
}

// conjugateInvolution conjugates the monster
// involution g to a standard representative z via up
// to ntrials random conjugations, returning (I, h,
// ok) with h^-1 g h = z. I is 0 for the identity, 1
// for a 2A involution, and 2 for a 2B involution. It
// returns ok=false if no conjugating element is found
// within the trial budget. It mirrors
// mm_conjugate_involution; failure is reported via ok
// rather than the broad ValueError/AssertionError
// catch in the Python source.
//
// conjugateInvolution panics if check is true and g
// is not an involution in the monster group.
func conjugateInvolution(g *MM, check bool, ntrials int) (int, *MM, bool) {
	g.Reduce()
	z := MMGen("x", 0x1000)
	one := MMIdentity()
	if check && !g.Mul(g).Equal(one) {
		panic("cgt: element is not an involution in the monster")
	}
	if h := g.checkInGx0(); h != nil {
		elem := NewXsp2Co1(atomsFromWord(h)...)
		it, hx := elem.ConjugateInvolution()
		return it, hx, true
	}
	for i := 0; i < ntrials; i++ {
		var s *MM
		if i == 0 {
			s = one
		} else {
			rounds := 3
			if r := i >> 2; r > rounds {
				rounds = r
			}
			s = MMRand(1 + rounds)
		}
		x := s.Inv().Mul(g).Mul(s)
		o, y := x.Mul(z).HalfOrder()
		if o == 0 || o&1 != 0 || y == nil {
			continue
		}
		// y = (x z)^(o/2) is an involution in G_x0
		// commuting with x and z.
		hy := y.checkInGx0()
		if hy == nil {
			continue
		}
		itype, h1, ok := conjugateInvolutionGx0(hy)
		if !ok || itype != 2 {
			continue
		}
		// x1 = x^h1 commutes with y^h1 = z, so x1 is in
		// G_x0.
		x1 := h1.Inv().Mul(x).Mul(h1)
		hx1 := x1.checkInGx0()
		if hx1 == nil {
			continue
		}
		itype2, h2, ok := conjugateInvolutionGx0(hx1)
		if !ok {
			continue
		}
		// x1^h2 = g^(s h1 h2) = z.
		t := s.Mul(h1).Mul(h2)
		t.Reduce()
		return itype2, t, true
	}
	return 0, nil, false
}

// conjugateInvolutionGx0 conjugates the G_x0
// involution given by the word w (in G_x0 atoms) to
// its standard representative, returning the type, a
// conjugating monster element, and ok=false if w is
// not an involution (mirroring the caught exception
// in mmgroup). w comes from MM.checkInGx0, so it is
// always a word in G_x0 generators; the only panic
// the conjugation path can raise here is the
// "not an involution" panic, which the trial loop
// must treat as a skip. Any other panic is a genuine
// bug and is re-raised.
func conjugateInvolutionGx0(w []uint32) (itype int, h *MM, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			if s, isStr := r.(string); isStr && s == errNotInvolution {
				itype, h, ok = 0, nil, false
				return
			}
			panic(r)
		}
	}()
	elem := NewXsp2Co1(atomsFromWord(w)...)
	it, hx := elem.ConjugateInvolution()
	return it, hx, true
}
