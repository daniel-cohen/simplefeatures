package simplefeatures

import (
	"fmt"
	"sort"
)

type threePointOrientation int

const (
	// rightTurn indicates the orientation is right turn which is anticlockwise
	rightTurn threePointOrientation = iota + 1
	// collinear indicates three points are on the same line
	collinear
	// leftTurn indicates the orientation is left turn which is clockwise
	leftTurn
)

func (o threePointOrientation) String() string {
	switch o {
	case rightTurn:
		return "right turn"
	case collinear:
		return "collinear"
	case leftTurn:
		return "left turn"
	default:
		return "invalid orientation"
	}
}

// orientation checks if s is on the right hand side or left hand side of the line formed by p and q.
func orientation(p, q, s XY) threePointOrientation {
	cp := q.Sub(p).Cross(s.Sub(q))
	switch {
	case cp.GT(zero):
		return leftTurn
	case cp.LT(zero):
		return rightTurn
	default:
		return collinear
	}
}

func convexHull(g Geometry) Geometry {
	if g.IsEmpty() {
		// special case to mirror postgis behaviour
		return g
	}
	pts := g.convexHullPointSet()
	hull := grahamScan(pts)
	switch len(hull) {
	case 0:
		return NewGeometryCollection(nil)
	case 1:
		return NewPoint(hull[0])
	case 2:
		ln, err := NewLine(
			Coordinates{hull[0]},
			Coordinates{hull[1]},
		)
		if err != nil {
			panic("bug in grahamScan routine - output 2 coincident points")
		}
		return ln
	default:
		coords := [][]Coordinates{make([]Coordinates, len(hull))}
		for i := range hull {
			coords[0][i] = Coordinates{XY: hull[i]}
		}
		poly, err := NewPolygonFromCoords(coords)
		if err != nil {
			panic(fmt.Errorf("bug in grahamScan routine - didn't produce a valid polygon: %v", err))
		}
		return poly
	}
}

type pointStack []XY

func (s *pointStack) push(p XY) {
	(*s) = append(*s, p)
}

func (s *pointStack) pop() XY {
	p := s.top()
	(*s) = (*s)[:len(*s)-1]
	return p
}

func (s *pointStack) top() XY {
	return (*s)[len(*s)-1]
}

func (s *pointStack) underTop() XY {
	return (*s)[len(*s)-2]
}

// grahamScan returns the convex hull of the input points. It will either
// represent the empty set (zero points), a point (one point), a line (2
// points), or a closed polygon (>= 3 points).
func grahamScan(ps []XY) []XY {
	if len(ps) <= 1 {
		return ps
	}

	sortByPolarAngle(ps)

	// Append the lowest-then-leftmost point so that the polygon will be closed.
	ps = append(ps, ps[0])

	// Populate the stack with the first 2 distict points.
	var i int // tracks progress through the ps slice
	var stack pointStack
	stack.push(ps[0])
	i++
	for i < len(ps) && len(stack) < 2 {
		if !stack.top().Equals(ps[i]) {
			stack.push(ps[i])
		}
		i++
	}

	for i < len(ps) {
		ori := orientation(stack.underTop(), stack.top(), ps[i])
		switch ori {
		case leftTurn:
			// This point _might_ be part of the convex hull. It will be popped
			// later if it actually isn't part of the convex hull.
			stack.push(ps[i])
		case collinear:
			// This point is part of the convex hull, so long as it extends the
			// current line segment (in which case the preceding point is
			// _not_ part of the convex hull).
			if distanceSq(stack.underTop(), ps[i]).GT(distanceSq(stack.underTop(), stack.top())) {
				stack.pop()
				stack.push(ps[i])
			}
		default:
			//  The preceding point was _not_ part of the convex hull (so it is
			//  popped). Potentially the new point is just an extension of a
			//  straight line (so pop the preceding point in that case so as to
			//  eliminate collinear points).
			stack.pop()
			if orientation(stack.underTop(), stack.top(), ps[i]) == collinear {
				stack.pop()
			}
			stack.push(ps[i])
		}
		i++
	}
	return stack
}

// sortByPolarAngle sorts the points by their polar angle relative to the
// lowest-then-leftmost anchor point.
func sortByPolarAngle(ps []XY) {
	// the lowest-then-leftmost (anchor) point comes first
	ltlp := ltl(ps)
	ps[ltlp], ps[0] = ps[0], ps[ltlp]
	anchor := ps[0]

	ps = ps[1:] // only sort the remaining points
	sort.Slice(ps, func(i, j int) bool {
		// If any point is equal to the anchor point, then always put it first.
		// This allows those duplicated points to be removed when the results
		// stack is initiated.
		if anchor.Equals(ps[i]) {
			return true
		}
		if anchor.Equals(ps[j]) {
			return false
		}
		// In the normal case, check which order the points are in relative to
		// the anchor.
		return orientation(anchor, ps[i], ps[j]) == leftTurn
	})
}

// ltl finds the index of the lowest-then-leftmost point.
func ltl(ps []XY) int {
	rpi := 0
	for i := 1; i < len(ps); i++ {
		if ps[i].Y.LT(ps[rpi].Y) ||
			(ps[i].Y.Equals(ps[rpi].Y) &&
				ps[i].X.LT(ps[rpi].X)) {
			rpi = i
		}
	}
	return rpi
}

// distanceSq gives the square of the distance between p and q.
func distanceSq(p, q XY) Scalar {
	pSubQ := p.Sub(q)
	return pSubQ.Dot(pSubQ)
}