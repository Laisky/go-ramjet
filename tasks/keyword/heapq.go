package keyword

import (
	"container/heap"

	utils "github.com/Laisky/go-utils"
)

type Item struct {
	priority int
	key      string
	idx      int
}

type PriorityQ []*Item

func (this PriorityQ) Len() int {
	return len(this)
}

func (this PriorityQ) Less(i, j int) bool {
	return this[i].priority > this[j].priority
}

func (this PriorityQ) Swap(i, j int) {
	this[i], this[j] = this[j], this[i]
	this[i].idx = i
	this[j].idx = j
}

func (this *PriorityQ) Push(x interface{}) {
	n := len(*this)
	item := x.(*Item)
	item.idx = n
	*this = append(*this, item)
}

func (this *PriorityQ) Pop() (popped interface{}) {
	old := *this
	n := len(old)
	item := old[n-1]
	item.idx = -1
	*this = old[0 : n-1]
	return item
}

func GetMostFreqWords(wordsMap map[string]int, topN int) (words []string) {
	utils.Logger.Debugf("GetMostFreqWords for wordsMap length %v, topN %v", len(wordsMap), topN)
	if len(wordsMap) < 2 {
		return words
	}

	pq := make(PriorityQ, len(wordsMap))
	i := 0
	for w, p := range wordsMap {
		pq[i] = &Item{
			priority: p,
			key:      w,
		}
		i++
	}

	heap.Init(&pq)
	var item *Item
	for i = 0; i < len(wordsMap); i++ {
		item = heap.Pop(&pq).(*Item)
		words = append(words, item.key)
		if i == topN {
			break
		}
	}

	return
}
