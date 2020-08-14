package geom

import (
	"fmt"
	"math"

	"github.com/peterstace/simplefeatures/rtree"
)

func distance(g1, g2 Geometry) (float64, bool) {
	// If the geometries intersect with each other, then the distance between
	// them is trivially zero.
	if g1.Intersects(g2) {
		return 0, true
	}

	// The general approach of the distance algorithm is as follows:
	//
	// 1. Convert both geometries to lists of points and line segments.
	//
	// 2. Index the second geometry using an RTree.
	//
	// 3. Iterate over every part (point or line) in the first geometry. For
	//    each part, search in the RTree for the nearest part of the second
	//    geometry. We can stop searching if the bounding box in the RTree is
	//    further away than the best distance so far.

	// TODO: Swap g1 and g2 so that the indexed geometry is the "smaller" one.
	// This is a performance optimization, so would need a benchmark to
	// justify.

	xys1, lns1 := extractXYsAndLines(g1)
	xys2, lns2 := extractXYsAndLines(g2)
	tr := loadTree(xys2, lns2)
	minDist := math.Inf(+1)

	searchBody := func(
		env Envelope,
		recordID int,
		xyDist func(int) float64,
		lnDist func(int) float64,
	) error {
		// Convert recordID back to array indexes.
		xyIdx := recordID - 1
		lnIdx := -recordID - 1

		// Abort the search if we're gone further away compared to our best
		// distance so far.
		var recordEnv Envelope
		if recordID > 0 {
			recordEnv = NewEnvelope(xys2[xyIdx])
		} else {
			recordEnv = lns2[lnIdx].envelope()
		}
		if recordEnv.Distance(env) > minDist {
			return rtree.Stop
		}

		// See if the current item in the tree is better than our current best
		// distance.
		if recordID > 0 {
			minDist = math.Min(minDist, xyDist(xyIdx))
		} else {
			minDist = math.Min(minDist, lnDist(lnIdx))
		}
		return nil
	}
	for _, xy := range xys1 {
		xyEnv := NewEnvelope(xy)
		tr.PrioritySearch(xyEnv.box(), func(recordID int) error {
			return searchBody(
				xyEnv,
				recordID,
				func(i int) float64 { return distBetweenXYs(xy, xys2[i]) },
				func(i int) float64 { return distBetweenXYAndLine(xy, lns2[i]) },
			)
		})
	}
	for _, ln := range lns1 {
		lnEnv := ln.envelope()
		tr.PrioritySearch(lnEnv.box(), func(recordID int) error {
			return searchBody(
				lnEnv,
				recordID,
				func(i int) float64 { return distBetweenXYAndLine(xys2[i], ln) },
				func(i int) float64 { return distBetweenLineAndLine(lns2[i], ln) },
			)
		})
	}

	if math.IsInf(minDist, +1) {
		return 0, false
	}
	return minDist, true
}

func extractXYsAndLines(g Geometry) ([]XY, []line) {
	switch g.Type() {
	case TypePoint:
		return g.AsPoint().asXYs(), nil
	case TypeLineString:
		return nil, g.AsLineString().asLines()
	case TypePolygon:
		return nil, g.AsPolygon().Boundary().asLines()
	case TypeMultiPoint:
		return g.AsMultiPoint().asXYs(), nil
	case TypeMultiLineString:
		return nil, g.AsMultiLineString().asLines()
	case TypeMultiPolygon:
		return nil, g.AsMultiPolygon().Boundary().asLines()
	case TypeGeometryCollection:
		var allXYs []XY
		var allLines []line
		g.AsGeometryCollection().walk(func(child Geometry) {
			xys, lns := extractXYsAndLines(child)
			allXYs = append(allXYs, xys...)
			allLines = append(allLines, lns...)
		})
		return allXYs, allLines
	default:
		panic(fmt.Sprintf("implementation error: unhandled geometry types %s", g.Type()))
	}
}

// loadTree creates a new RTree that indexes both the XYs and the lines. It
// uses positive record IDs to refer to the XYs, and negative recordIDs to
// refer to the lines. Because +0 and -0 are the same, indexing is 1-based and
// recordID 0 is not used.
func loadTree(xys []XY, lns []line) *rtree.RTree {
	items := make([]rtree.BulkItem, len(xys)+len(lns))
	for i, xy := range xys {
		items[i] = rtree.BulkItem{
			Box:      NewEnvelope(xy).box(),
			RecordID: i + 1,
		}
	}
	for i, ln := range lns {
		items[i+len(xys)] = rtree.BulkItem{
			Box:      ln.envelope().box(),
			RecordID: -(i + 1),
		}
	}
	return rtree.BulkLoad(items)
}

func distBetweenXYs(xy1, xy2 XY) float64 {
	return xy1.Sub(xy2).Length()
}

func distBetweenXYAndLine(xy XY, ln line) float64 {
	ab := ln.b.Sub(ln.a)
	abLen := ab.Length()
	proj := xy.Sub(ln.a).Dot(ab) / abLen
	var closest XY
	switch {
	case proj < 0:
		closest = ln.a
	case proj > abLen:
		closest = ln.b
	default:
		scaled := ab.Scale(proj / abLen)
		closest = scaled.Add(ln.a)
	}
	return distBetweenXYs(xy, closest)
}

func distBetweenLineAndLine(ln1, ln2 line) float64 {
	minDist := math.Inf(+1)
	for _, dist := range [4]float64{
		distBetweenXYAndLine(ln1.a, ln2),
		distBetweenXYAndLine(ln1.b, ln2),
		distBetweenXYAndLine(ln2.a, ln1),
		distBetweenXYAndLine(ln2.b, ln1),
	} {
		minDist = math.Min(minDist, dist)
	}
	return minDist
}
