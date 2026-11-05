// Package contract supplies isolated synthetic protocol evidence for review.
package contract

import "crypto/ed25519"

type State struct {
	Switch string
	Version uint32
	Sequence uint64
	Deadline int64
	Grace uint64
	Content, Key, Enrollment [32]byte
}

type Entry struct { ID string; Key [32]byte; Data []byte }
type Message struct {
	Operation, Signer string
	State State
	Nonce [32]byte
	Durable uint64
	Phase byte
	Released bool
	Entries []Entry
}
type Envelope struct { Body, Signature []byte }

func encode(m Message) ([]byte,error) { return nil,nil }
func verify(e Envelope, keys map[string]ed25519.PublicKey) (Message,error) { return Message{},nil }
func journal(e Envelope, claimed State, keys map[string]ed25519.PublicKey) error { return nil }
func accept(old State, next State) error { return nil }

type Snapshot struct { State State; Phase byte; Released, Unsafe bool; Generation uint64 }
type Authority struct { Head Snapshot; Complete bool; Present bool }
func restore(s Snapshot, a Authority) error { return nil }
func transition(s Snapshot, action string, durable bool, fresh bool) (Snapshot,bool,error) { return s,false,nil }
