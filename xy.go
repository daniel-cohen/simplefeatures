package simplefeatures

import (
	"crypto/sha256"
	"fmt"
)

type XY struct {
	X, Y Scalar
}

func (w XY) Equals(o XY) bool {
	return w.X.Equals(o.X) && w.Y.Equals(o.Y)
}

func (w XY) Sub(o XY) XY {
	return XY{
		w.X.Sub(o.X),
		w.Y.Sub(o.Y),
	}
}

func (w XY) Add(o XY) XY {
	return XY{
		w.X.Add(o.X),
		w.Y.Add(o.Y),
	}
}

func (w XY) Scale(s Scalar) XY {
	return XY{
		w.X.Mul(s),
		w.Y.Mul(s),
	}
}

func (w XY) Cross(o XY) Scalar {
	return w.X.Mul(o.Y).Sub(w.Y.Mul(o.X))
}

func (w XY) Midpoint(o XY) XY {
	return w.Add(o).Scale(half)
}

type xyHash [sha256.Size]byte

func (w XY) hash() xyHash {
	h := sha256.New()
	fmt.Fprintf(h, "%s,%s", w.X.val, w.Y.val)
	var sum xyHash
	h.Sum(sum[:0])
	return sum
}

type xySet map[xyHash]XY

func newXYSet() xySet {
	return make(map[xyHash]XY)
}

func (s xySet) add(xy XY) {
	s[xy.hash()] = xy
}

type xyxyHash struct {
	a, b xyHash
}

func hashXYXY(a, b XY) xyxyHash {
	return xyxyHash{
		a.hash(),
		b.hash(),
	}
}