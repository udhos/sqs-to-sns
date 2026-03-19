package main

import "testing"

// go test -count 1 -run '^TestPool$' ./...
func TestPool(t *testing.T) {
	p := newPool()

	{
		m := p.getAvailable()
		if len(m) != 0 {
			t.Fatalf("expecting empty getAvailable: %v", m)
		}
	}

	p.add(message{})

	{
		m := p.getAvailable()
		if len(m) != 1 {
			t.Fatalf("expecting 1 getAvailable: %v", m)
		}
	}

	p.add(message{})
	p.add(message{})

	{
		m := p.getAvailable()
		if len(m) != 2 {
			t.Fatalf("expecting 2 getAvailable: %v", m)
		}
	}

	for range 11 {
		p.add(message{})
	}

	{
		m := p.getAvailable()
		if len(m) != 10 {
			t.Fatalf("expecting 10 getAvailable: %v", m)
		}
	}

	// 1 message

	{
		_, found := p.getFullBatch()
		if found {
			t.Fatal("expecting not found")
		}
	}

	// 1 message

	for range 9 {
		p.add(message{})
	}

	// 10 messages

	{
		m, found := p.getFullBatch()
		if !found {
			t.Fatal("expecting found")
		}
		if len(m) != 10 {
			t.Fatal("expecting full batch")
		}
	}

	// 0 messages

	{
		m := p.getAvailable()
		if len(m) != 0 {
			t.Fatalf("expecting 0 getAvailable: %v", m)
		}
	}

}
