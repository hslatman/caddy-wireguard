package tcpip

// ElementMapper provides an identity mapping by default.
//
// This can be replaced to provide a struct that maps elements to linker
// objects, if they are not the same. An ElementMapper is not typically
// required if: Linker is left as is, Element is left as is, or Linker and
// Element are the same type.
type sockErrorElementMapper struct{}

// linkerFor maps an Element to a Linker.
//
// This default implementation should be inlined.
//
//go:nosplit
func (sockErrorElementMapper) linkerFor(elem *SockError) *SockError { return elem }

// List is an intrusive list. Entries can be added to or removed from the list
// in O(1) time and with no additional memory allocations.
//
// The zero value for List is an empty list ready to use.
//
// To iterate over a list (where l is a List):
//      for e := l.Front(); e != nil; e = e.Next() {
// 		// do something with e.
//      }
//
// +stateify savable
type sockErrorList struct {
	head *SockError
	tail *SockError
}

// Reset resets list l to the empty state.
func (l *sockErrorList) Reset() {
	l.head = nil
	l.tail = nil
}

// Empty returns true iff the list is empty.
func (l *sockErrorList) Empty() bool {
	return l.head == nil
}

// Front returns the first element of list l or nil.
func (l *sockErrorList) Front() *SockError {
	return l.head
}

// Back returns the last element of list l or nil.
func (l *sockErrorList) Back() *SockError {
	return l.tail
}

// Len returns the number of elements in the list.
//
// NOTE: This is an O(n) operation.
func (l *sockErrorList) Len() (count int) {
	for e := l.Front(); e != nil; e = (sockErrorElementMapper{}.linkerFor(e)).Next() {
		count++
	}
	return count
}

// PushFront inserts the element e at the front of list l.
func (l *sockErrorList) PushFront(e *SockError) {
	linker := sockErrorElementMapper{}.linkerFor(e)
	linker.SetNext(l.head)
	linker.SetPrev(nil)
	if l.head != nil {
		sockErrorElementMapper{}.linkerFor(l.head).SetPrev(e)
	} else {
		l.tail = e
	}

	l.head = e
}

// PushBack inserts the element e at the back of list l.
func (l *sockErrorList) PushBack(e *SockError) {
	linker := sockErrorElementMapper{}.linkerFor(e)
	linker.SetNext(nil)
	linker.SetPrev(l.tail)
	if l.tail != nil {
		sockErrorElementMapper{}.linkerFor(l.tail).SetNext(e)
	} else {
		l.head = e
	}

	l.tail = e
}

// PushBackList inserts list m at the end of list l, emptying m.
func (l *sockErrorList) PushBackList(m *sockErrorList) {
	if l.head == nil {
		l.head = m.head
		l.tail = m.tail
	} else if m.head != nil {
		sockErrorElementMapper{}.linkerFor(l.tail).SetNext(m.head)
		sockErrorElementMapper{}.linkerFor(m.head).SetPrev(l.tail)

		l.tail = m.tail
	}
	m.head = nil
	m.tail = nil
}

// InsertAfter inserts e after b.
func (l *sockErrorList) InsertAfter(b, e *SockError) {
	bLinker := sockErrorElementMapper{}.linkerFor(b)
	eLinker := sockErrorElementMapper{}.linkerFor(e)

	a := bLinker.Next()

	eLinker.SetNext(a)
	eLinker.SetPrev(b)
	bLinker.SetNext(e)

	if a != nil {
		sockErrorElementMapper{}.linkerFor(a).SetPrev(e)
	} else {
		l.tail = e
	}
}

// InsertBefore inserts e before a.
func (l *sockErrorList) InsertBefore(a, e *SockError) {
	aLinker := sockErrorElementMapper{}.linkerFor(a)
	eLinker := sockErrorElementMapper{}.linkerFor(e)

	b := aLinker.Prev()
	eLinker.SetNext(a)
	eLinker.SetPrev(b)
	aLinker.SetPrev(e)

	if b != nil {
		sockErrorElementMapper{}.linkerFor(b).SetNext(e)
	} else {
		l.head = e
	}
}

// Remove removes e from l.
func (l *sockErrorList) Remove(e *SockError) {
	linker := sockErrorElementMapper{}.linkerFor(e)
	prev := linker.Prev()
	next := linker.Next()

	if prev != nil {
		sockErrorElementMapper{}.linkerFor(prev).SetNext(next)
	} else if l.head == e {
		l.head = next
	}

	if next != nil {
		sockErrorElementMapper{}.linkerFor(next).SetPrev(prev)
	} else if l.tail == e {
		l.tail = prev
	}

	linker.SetNext(nil)
	linker.SetPrev(nil)
}

// Entry is a default implementation of Linker. Users can add anonymous fields
// of this type to their structs to make them automatically implement the
// methods needed by List.
//
// +stateify savable
type sockErrorEntry struct {
	next *SockError
	prev *SockError
}

// Next returns the entry that follows e in the list.
func (e *sockErrorEntry) Next() *SockError {
	return e.next
}

// Prev returns the entry that precedes e in the list.
func (e *sockErrorEntry) Prev() *SockError {
	return e.prev
}

// SetNext assigns 'entry' as the entry that follows e in the list.
func (e *sockErrorEntry) SetNext(elem *SockError) {
	e.next = elem
}

// SetPrev assigns 'entry' as the entry that precedes e in the list.
func (e *sockErrorEntry) SetPrev(elem *SockError) {
	e.prev = elem
}