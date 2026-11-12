package signwell

import "context"

// Iterator walks every item of a paged list, fetching the next page when the
// current one runs out. Use it like bufio.Scanner:
//
//	it := client.Documents.ListAll(&signwell.ListParams{Limit: 50})
//	for it.Next(ctx) {
//		doc := it.Current()
//		fmt.Println(doc.ID, doc.Status)
//	}
//	if err := it.Err(); err != nil {
//		return err
//	}
//
// An Iterator is not safe for concurrent use.
type Iterator[T any] struct {
	fetch func(ctx context.Context, page int) ([]T, Pagination, error)

	page    int
	items   []T
	idx     int
	cur     T
	err     error
	done    bool
	fetched int
	last    Pagination
}

func newIterator[T any](start int, fetch func(ctx context.Context, page int) ([]T, Pagination, error)) *Iterator[T] {
	if start < 1 {
		start = 1
	}
	return &Iterator[T]{fetch: fetch, page: start}
}

// Next moves to the next item and reports whether there is one. It returns
// false at the end of the list or on the first error, see Err.
func (it *Iterator[T]) Next(ctx context.Context) bool {
	for {
		if it.idx < len(it.items) {
			it.cur = it.items[it.idx]
			it.idx++
			return true
		}
		if it.done || it.err != nil {
			return false
		}
		items, pg, err := it.fetch(ctx, it.page)
		if err != nil {
			it.err = err
			return false
		}
		it.fetched++
		it.last = pg
		it.items, it.idx = items, 0
		// Stop when the API reports no next page, or a next page that does not
		// move forward, so a bad response cannot loop forever.
		if pg.NextPage == nil || *pg.NextPage <= it.page {
			it.done = true
		} else {
			it.page = *pg.NextPage
		}
	}
}

// Current returns the item Next moved to.
func (it *Iterator[T]) Current() T { return it.cur }

// Err returns the error that stopped the iteration, if any.
func (it *Iterator[T]) Err() error { return it.err }

// Pages returns how many pages have been fetched so far.
func (it *Iterator[T]) Pages() int { return it.fetched }

// TotalCount returns the total number of items the API reported on the
// last fetched page, or zero before the first fetch.
func (it *Iterator[T]) TotalCount() int { return it.last.TotalCount }

// Collect reads the remaining items into a slice. It stops at the first
// error and returns the items read before it.
func (it *Iterator[T]) Collect(ctx context.Context) ([]T, error) {
	var out []T
	for it.Next(ctx) {
		out = append(out, it.Current())
	}
	return out, it.Err()
}
