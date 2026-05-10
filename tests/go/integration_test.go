package main

import (
	"testing"
	"time"

	sdk "github.com/atoncooper/mcache/sdk/go"
)

const testAddr = "127.0.0.1:11211"

func newTestClient(t *testing.T) *sdk.Client {
	t.Helper()
	c, err := sdk.NewClient(testAddr,
		sdk.WithPoolSize(2),
		sdk.WithDialTimeout(5*time.Second),
		sdk.WithReadTimeout(10*time.Second),
		sdk.WithWriteTimeout(5*time.Second),
		sdk.WithCodec(sdk.RawCodec{}),
	)
	if err != nil {
		t.Fatalf("connect to %s: %v", testAddr, err)
	}
	return c
}

func TestSetAndGet(t *testing.T) {
	c := newTestClient(t)
	defer c.Close()

	key := "test:set_get"
	value := "hello-sdk"

	if err := c.Set(key, value, time.Minute); err != nil {
		t.Fatalf("set failed: %v", err)
	}

	var got string
	if err := c.Get(key, &got); err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if got != value {
		t.Fatalf("expected %q, got %q", value, got)
	}
}

func TestDel(t *testing.T) {
	c := newTestClient(t)
	defer c.Close()

	key := "test:del"
	if err := c.Set(key, "v", time.Minute); err != nil {
		t.Fatalf("set failed: %v", err)
	}

	if err := c.Del(key); err != nil {
		t.Fatalf("del failed: %v", err)
	}

	var got string
	if err := c.Get(key, &got); err == nil {
		t.Fatalf("expected error after delete, got %q", got)
	}
}

func TestLen(t *testing.T) {
	c := newTestClient(t)
	defer c.Close()

	key := "test:len"
	if err := c.Set(key, "v", time.Minute); err != nil {
		t.Fatalf("set failed: %v", err)
	}

	n, err := c.Len()
	if err != nil {
		t.Fatalf("len failed: %v", err)
	}
	if n < 1 {
		t.Fatalf("expected len >= 1, got %d", n)
	}
}
