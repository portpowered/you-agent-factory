// Package service is the in-memory, concurrency-safe implementation of the
// chatsessions.Service root contract. It is owned by chat_sessions and
// composed only through chat_sessions/wire; nothing outside chat_sessions
// imports this package directly.
//
// Store holds every mutable aggregate behind one mutex and returns detached
// (copied) chatsessions values, so a caller can never observe or corrupt
// another caller's in-progress mutation. Store keeps only in-memory state:
// it performs no filesystem, database, recorder, or other durable-store
// writes, and two independently constructed Store instances share no state.
package service
