package rtree

import (
	"sort"
)

// BulkItem is an item that can be inserted for bulk loading.
type BulkItem struct {
	Box      Box
	RecordID int
}

// BulkLoad bulk loads multiple items into a new R-Tree. The bulk load
// operation is optimised for creating R-Trees with minimal node overlap. This
// allows for fast searching.
func BulkLoad(items []BulkItem) *RTree {
	if len(items) == 0 {
		return &RTree{}
	}

	levels := calculateLevels(len(items))
	return &RTree{bulkInsert(items, levels)}
}

func calculateLevels(numItems int) int {
	// We could theoretically do this calculation using math.Log. However,
	// float precision issues can cause off-by-one errors in some scenarios.
	// Instead, we calculate the number of levels using integer arithmetic
	// only. This will be fast anyway, since the calculation only requires
	// logarithmic time.
	levels := 1
	count := maxChildren
	for count < numItems {
		count *= maxChildren
		levels++
	}
	return levels
}

func bulkInsert(items []BulkItem, levels int) *node {
	if levels == 1 {
		root := &node{isLeaf: true, numEntries: len(items)}
		for i, item := range items {
			root.entries[i] = entry{
				box:      item.Box,
				recordID: item.RecordID,
			}
		}
		return root
	}

	// NOTE: bulk loading is hardcoded around the fact that the min and max
	// node cardinalities are 2 and 4.

	// 6 is the first number of items that can be split into 3 nodes while
	// respecting the minimum node cardinality, i.e. 6 = 2 + 2 + 2. Anything
	// less than 6 must instead be split into 2 nodes.
	if len(items) < 6 {
		firstHalf, secondHalf := splitBulkItems2Ways(items)
		return bulkNode(levels, firstHalf, secondHalf)
	}

	// 8 is the first number of items that can be split into 4 nodes while
	// respecting the minimum node cardinality, i.e. 8 = 2 + 2 + 2 + 2.
	// Anything less that 8 must instead be split into 3 nodes.
	if len(items) < 8 {
		firstThird, secondThird, thirdThird := splitBulkItems3Ways(items)
		return bulkNode(levels, firstThird, secondThird, thirdThird)
	}

	// 4-way split:
	firstHalf, secondHalf := splitBulkItems2Ways(items)
	firstQuarter, secondQuarter := splitBulkItems2Ways(firstHalf)
	thirdQuarter, fourthQuarter := splitBulkItems2Ways(secondHalf)
	return bulkNode(levels, firstQuarter, secondQuarter, thirdQuarter, fourthQuarter)
}

func bulkNode(levels int, parts ...[]BulkItem) *node {
	root := &node{
		numEntries: len(parts),
		parent:     nil,
		isLeaf:     false,
	}
	for i, part := range parts {
		child := bulkInsert(part, levels-1)
		child.parent = root
		root.entries[i].child = child
		root.entries[i].box = calculateBound(child)
	}
	return root
}

func splitBulkItems2Ways(items []BulkItem) ([]BulkItem, []BulkItem) {
	bulkItems := buildBulkItems(items)
	split := len(items) / 2
	quickPartition(bulkItems, split)
	return items[:split], items[split:]
}

func splitBulkItems3Ways(items []BulkItem) ([]BulkItem, []BulkItem, []BulkItem) {
	// We only need to split 3 ways when we have 6 or 7 elements. By making use
	// of that assumption, we greatly simplify the logic in this function
	// compared to if we were to implement a 3 way split in the general case.
	if ln := len(items); ln != 6 && ln != 7 {
		panic(len(items))
	}

	bulkItems := buildBulkItems(items)
	quickPartition(bulkItems, 2)
	bulkItems.items = bulkItems.items[3:]
	quickPartition(bulkItems, 1)

	return items[:2], items[2:4], items[4:]
}

// quickPartition performs a partial in-place sort on the items slice. The
// partial sort is such that items 0 through k-1 are less than or equal to item
// k, and items k+1 through n-1 are greater than or equal to item k.
func quickPartition(items sort.Interface, k int) {
	// Use a custom linear congruential random number generator. This is used
	// because we don't need high quality random numbers. Using a regular
	// rand.Rand generator causes a significant bottleneck due to the reliance
	// on random numbers in this algorithm.
	var rndState int
	rnd := func(n int) int {
		rndState = (1664525*rndState + 1013904223) % (1 << 32)
		return rndState % n
	}

	left, right := 0, items.Len()-1
	for {
		// Select pivot and store it at the end.
		pivot := left + rnd(right-left+1)
		items.Swap(pivot, right)

		// Partition the left and right sides of the pivot.
		j := left
		for i := left; i <= right; i++ {
			if items.Less(i, right) {
				items.Swap(i, j)
				j++
			}
		}

		// Restore the pivot to the middle position between the two partitions.
		items.Swap(right, j)

		// Repeat on either the left or right parts depending on which contains
		// the kth element.
		switch {
		case j-left < k:
			k -= j - left + 1
			left = j + 1
		case j-left > k:
			right = j - 1
		default:
			return
		}
	}
}

func buildBulkItems(items []BulkItem) bulkItems {
	box := items[0].Box
	for _, item := range items[1:] {
		box = combine(box, item.Box)
	}
	return bulkItems{
		horizontal: box.MaxX-box.MinX > box.MaxY-box.MinY,
		items:      items,
	}
}

// bulkItems implements the sort.Interface interface. This style of sorting is
// used rather than sort.Slice because it does less allocations.
type bulkItems struct {
	horizontal bool
	items      []BulkItem
}

func (b bulkItems) Len() int {
	return len(b.items)
}
func (b bulkItems) Less(i, j int) bool {
	bi := b.items[i].Box
	bj := b.items[j].Box
	if b.horizontal {
		return bi.MinX+bi.MaxX < bj.MinX+bj.MaxX
	} else {
		return bi.MinY+bi.MaxY < bj.MinY+bj.MaxY
	}
}
func (b bulkItems) Swap(i, j int) {
	b.items[i], b.items[j] = b.items[j], b.items[i]
}
