package signwell

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

// pagedSource fakes a paged endpoint from a list of pages.
type pagedSource struct {
	pages   map[int][]int
	next    map[int]*int
	failAt  int
	fetches []int
}

func (s *pagedSource) fetch(_ context.Context, page int) ([]int, Pagination, error) {
	s.fetches = append(s.fetches, page)
	if page == s.failAt {
		return nil, Pagination{}, errors.New("boom")
	}
	return s.pages[page], Pagination{CurrentPage: page, NextPage: s.next[page], TotalCount: 99}, nil
}

func TestIterator(t *testing.T) {
	tests := []struct {
		name        string
		src         *pagedSource
		start       int
		want        []int
		wantFetches []int
		wantErr     bool
	}{
		{
			name:        "single page",
			src:         &pagedSource{pages: map[int][]int{1: {1, 2}}},
			want:        []int{1, 2},
			wantFetches: []int{1},
		},
		{
			name:        "three pages",
			src:         &pagedSource{pages: map[int][]int{1: {1}, 2: {2}, 3: {3}}, next: map[int]*int{1: Int(2), 2: Int(3)}},
			want:        []int{1, 2, 3},
			wantFetches: []int{1, 2, 3},
		},
		{
			name:        "empty list",
			src:         &pagedSource{pages: map[int][]int{}},
			want:        nil,
			wantFetches: []int{1},
		},
		{
			name:        "empty page in the middle",
			src:         &pagedSource{pages: map[int][]int{1: {1}, 3: {3}}, next: map[int]*int{1: Int(2), 2: Int(3)}},
			want:        []int{1, 3},
			wantFetches: []int{1, 2, 3},
		},
		{
			name:        "next page that does not advance stops",
			src:         &pagedSource{pages: map[int][]int{1: {1}, 2: {2}}, next: map[int]*int{1: Int(2), 2: Int(2)}},
			want:        []int{1, 2},
			wantFetches: []int{1, 2},
		},
		{
			name:        "next page going back stops",
			src:         &pagedSource{pages: map[int][]int{1: {1}}, next: map[int]*int{1: Int(1)}},
			want:        []int{1},
			wantFetches: []int{1},
		},
		{
			name:        "start page",
			src:         &pagedSource{pages: map[int][]int{3: {30}, 4: {40}}, next: map[int]*int{3: Int(4)}},
			start:       3,
			want:        []int{30, 40},
			wantFetches: []int{3, 4},
		},
		{
			name:        "error on second page keeps first page items",
			src:         &pagedSource{pages: map[int][]int{1: {1, 2}}, next: map[int]*int{1: Int(2)}, failAt: 2},
			want:        []int{1, 2},
			wantFetches: []int{1, 2},
			wantErr:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			it := newIterator(tt.start, tt.src.fetch)
			got, err := it.Collect(context.Background())
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("items = %v, want %v", got, tt.want)
			}
			if !reflect.DeepEqual(tt.src.fetches, tt.wantFetches) {
				t.Errorf("fetches = %v, want %v", tt.src.fetches, tt.wantFetches)
			}
		})
	}
}

func TestIteratorAfterEnd(t *testing.T) {
	src := &pagedSource{pages: map[int][]int{1: {7}}}
	it := newIterator(1, src.fetch)
	ctx := context.Background()
	if !it.Next(ctx) || it.Current() != 7 {
		t.Fatal("first item missing")
	}
	for i := 0; i < 3; i++ {
		if it.Next(ctx) {
			t.Fatal("Next returned true after the end")
		}
	}
	if len(src.fetches) != 1 || it.Pages() != 1 || it.TotalCount() != 99 {
		t.Errorf("fetches %v pages %d total %d", src.fetches, it.Pages(), it.TotalCount())
	}
}

func TestIteratorStopsAfterError(t *testing.T) {
	src := &pagedSource{failAt: 1}
	it := newIterator(1, src.fetch)
	ctx := context.Background()
	if it.Next(ctx) || it.Next(ctx) {
		t.Fatal("Next returned true after an error")
	}
	if it.Err() == nil || len(src.fetches) != 1 {
		t.Errorf("err %v fetches %v", it.Err(), src.fetches)
	}
}
