package raft

// LogEntry is a single command in the replicated log.
type LogEntry struct {
	Index   uint64 `json:"index"`
	Term    uint64 `json:"term"`
	Command []byte `json:"command"`
}

// Log is an in-memory replicated log.
type Log struct {
	entries []LogEntry
}

// NewLog creates an empty log. Index 0 is a dummy entry to simplify 1-based indexing.
func NewLog() *Log {
	return &Log{
		entries: []LogEntry{{Index: 0, Term: 0}},
	}
}

// Append adds entries to the log.
func (l *Log) Append(entries ...LogEntry) {
	l.entries = append(l.entries, entries...)
}

// Get returns the entry at the given index (1-based).
func (l *Log) Get(index uint64) (LogEntry, bool) {
	if index == 0 || index >= uint64(len(l.entries)) {
		return LogEntry{}, false
	}
	return l.entries[index], true
}

// Last returns the last index and term.
func (l *Log) Last() (index, term uint64) {
	if len(l.entries) == 0 {
		return 0, 0
	}
	last := l.entries[len(l.entries)-1]
	return last.Index, last.Term
}

// Len returns the number of entries (excluding the dummy at index 0).
func (l *Log) Len() int {
	return len(l.entries) - 1
}

// Truncate discards all entries after the given index (inclusive).
func (l *Log) Truncate(from uint64) {
	if from < uint64(len(l.entries)) {
		l.entries = l.entries[:from]
	}
}

// Slice returns entries in the range [start, end).
func (l *Log) Slice(start, end uint64) []LogEntry {
	if start < 1 {
		start = 1
	}
	if end > uint64(len(l.entries)) {
		end = uint64(len(l.entries))
	}
	if start >= end {
		return nil
	}
	out := make([]LogEntry, end-start)
	copy(out, l.entries[start:end])
	return out
}
