package util

// Queue is just a basic FIFO container that automatically removes the oldest element
// whenever it becomes full.
type Queue struct {
	data    []string
	maxSize int
}

func NewQueue(maxSize int) *Queue {
	return &Queue{
		data:    make([]string, 0),
		maxSize: maxSize,
	}
}

func (q *Queue) Push(item string) {
	q.data = append(q.data, item)
	if len(q.data) > q.maxSize {
		q.data = q.data[1:]
	}
}

func (q *Queue) Contains(item string) bool {
	for _, s := range q.data {
		if s == item {
			return true
		}
	}
	return false
}
